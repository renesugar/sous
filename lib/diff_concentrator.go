package sous

import (
	"sort"

	"github.com/pkg/errors"
)

type (
	// ManifestPair is a pair of manifests.
	ManifestPair struct {
		name        ManifestID
		Prior, Post *Manifest
	}
	// ManifestPairs is a slice of *ManifestPair.
	ManifestPairs []*ManifestPair

	// A DiffConcentrator wraps deployment DeployableChans in order to produce
	// differences in terms of *manifests*
	DiffConcentrator struct {
		Defs
		baseManifests              Manifests
		Errors                     chan error
		Created, Deleted, Retained chan *Manifest
		Modified                   chan *ManifestPair
	}

	concentratedDiffSet struct {
		New, Gone, Same Manifests
		Changed         ManifestPairs
	}

	deploymentBundle struct {
		consumed bool
		before   Deployments
		after    Deployments
	}
)

func newConcDiffSet() concentratedDiffSet {
	return concentratedDiffSet{
		New:     NewManifests(),
		Gone:    NewManifests(),
		Same:    NewManifests(),
		Changed: make(ManifestPairs, 0),
	}
}

func (dc *DiffConcentrator) collect() (concentratedDiffSet, error) {
	ds := newConcDiffSet()

	select {
	default:
	case err := <-dc.Errors:
		return ds, err
	}
	for g := range dc.Deleted {
		ds.Gone.Add(g)
	}
	for n := range dc.Created {
		ds.New.Add(n)
	}
	for m := range dc.Modified {
		ds.Changed = append(ds.Changed, m)
	}
	for s := range dc.Retained {
		ds.Same.Add(s)
	}
	select {
	default:
	case err := <-dc.Errors:
		return ds, err
	}

	return ds, nil
}

func (db *deploymentBundle) add(pair *DeployablePair) error {
	var prior, post *Deployment

	if pair.Prior != nil {
		prior = pair.Prior.Deployment
	}

	if pair.Post != nil {
		post = pair.Post.Deployment
	}

	if prior == nil || post == nil {
		if prior == nil {
			Log.Debug.Printf("Added deployment: %#v", post)
		} else {
			Log.Debug.Printf("Depleted deployment: %#v", prior)
		}
	} else if different, diffs := post.Diff(prior); different {
		Log.Debug.Printf("Adding modification to deployment bundle (%q)", prior.ID())
		Log.Debug.Printf("Diffs for %q: % #v", prior.ID(), diffs)
	}

	if db.consumed {
		return errors.Errorf("Attempted to add a new pair to a consumed bundle: %v %v", prior, post)
	}
	var cluster string
	if prior != nil {
		cluster = prior.ClusterName
	}
	if post != nil {
		if prior == nil {
			cluster = post.ClusterName
		} else if cluster != post.ClusterName {
			return errors.Errorf("Invariant violated: two clusters named in deploy pair: %q vs %q", prior.ClusterName, post.ClusterName)
		}
	}
	if cluster == "" {
		return errors.Errorf("Invariant violated: no cluster name given in deploy pair")
	}

	if prior != nil {
		if accepted := db.before.Add(prior); !accepted {
			existing, present := db.before.Get(prior.ID())
			if !present {
				panic("Collided deployment not present!")
			}
			return errors.Errorf(
				"Deployment collision for cluster's prior %q:\n  %v vs\n  %v",
				cluster, existing, prior,
			)
		}
	}

	if post != nil {
		if accepted := db.after.Add(post); !accepted {
			existing, present := db.after.Get(post.ID())
			if !present {
				panic("Collided deployment not present!")
			}
			return errors.Errorf(
				"Deployment collision for cluster's post %q:\n  %v vs\n  %v",
				cluster, existing, post,
			)
		}
	}

	return nil
}

func (db *deploymentBundle) clusters() []string {
	cm := make(map[string]struct{})
	for _, v := range db.before.Snapshot() {
		cm[v.ClusterName] = struct{}{}
	}
	for _, v := range db.after.Snapshot() {
		cm[v.ClusterName] = struct{}{}
	}
	cs := make([]string, 0, len(cm))
	for k := range cm {
		cs = append(cs, k)
	}
	sort.Strings(cs)
	return cs
}

func (db *deploymentBundle) manifestPair(defs Defs, baseManifests Manifests) (*ManifestPair, error) {
	db.consumed = true
	//log.Print(db)
	res := new(ManifestPair)
	ms, err := db.before.PutbackManifests(defs, baseManifests)
	if err != nil {
		return nil, err
	}
	//log.Print(ms)
	p, err := ms.Only()
	if err != nil {
		return nil, err
	}
	if p != nil {
		res.Prior = p
	}

	ms, err = db.after.PutbackManifests(defs, baseManifests)
	if err != nil {
		return nil, err
	}
	p, err = ms.Only()
	if err != nil {
		return nil, err
	}
	if p != nil {
		res.Post = p
	}

	//log.Print(res)
	//log.Print(res.Prior)
	//log.Print(res.Post)

	if res.Post == nil {
		res.name = res.Prior.ID()
	} else {
		res.name = res.Post.ID()
	}

	return res, nil
}

func newDepBundle() *deploymentBundle {
	return &deploymentBundle{
		consumed: false,
		before:   NewDeployments(),
		after:    NewDeployments(),
	}
}

func (dc *DiffConcentrator) dispatch(mp *ManifestPair) error {
	if mp.Prior == nil {
		if mp.Post == nil {
			return errors.Errorf("Blank manifest pair: %#v", mp)
		}
		dc.Created <- mp.Post
		return nil
	}
	if mp.Post == nil {
		dc.Deleted <- mp.Prior
		return nil
	}
	if mp.Prior.Equal(mp.Post) {
		dc.Retained <- mp.Post
		return nil
	}
	dc.Modified <- mp
	return nil
}

func (dc *DiffConcentrator) resolve(mid ManifestID, bundle *deploymentBundle) {
	mp, err := bundle.manifestPair(dc.Defs, dc.baseManifests)
	if err != nil {
		dc.Errors <- err
		return
	}
	if err := dc.dispatch(mp); err != nil {
		dc.Errors <- err
	}
}

func concentrate(dc DeployableChans, con DiffConcentrator) {
	collect := make(map[ManifestID]*deploymentBundle)
	addPair := func(mid ManifestID, dp *DeployablePair) {
		_, present := collect[mid]
		if !present {
			collect[mid] = newDepBundle()
		}

		err := collect[mid].add(dp)
		if err != nil {
			con.Errors <- err
			return
		}

		Log.Debug.Printf("For %v, have %d clusters, waiting for %d", mid, len(collect[mid].clusters()), len(con.Defs.Clusters))
		if len(collect[mid].clusters()) == len(con.Defs.Clusters) { //eh?
			con.resolve(mid, collect[mid])
		}
	}

	created, deleted, retained, modified :=
		dc.Start, dc.Stop, dc.Stable, dc.Update

	defer func() {
		close(con.Retained)
		close(con.Modified)
		close(con.Errors)
		close(con.Created)
		close(con.Deleted)
	}()

	for {
		if created == nil && deleted == nil && retained == nil && modified == nil {
			break
		}

		select {
		case c, open := <-created:
			if !open {
				created = nil
				continue
			}
			addPair(c.Post.ManifestID(), c)
		case d, open := <-deleted:
			if !open {
				deleted = nil
				continue
			}
			addPair(d.Prior.ManifestID(), d)
		case r, open := <-retained:
			if !open {
				retained = nil
				continue
			}
			addPair(r.Post.ManifestID(), r)
		case m, open := <-modified:
			if !open {
				modified = nil
				continue
			}

			Log.Debug.Printf("Concentrating modification of %q", m.ID())

			addPair(m.Prior.ManifestID(), m)
		}
	}

	for mid, bundle := range collect {
		if !bundle.consumed {
			con.resolve(mid, collect[mid])
		}
	}
}
