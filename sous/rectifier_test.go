package sous

import (
	"log"
	"testing"

	"github.com/samsalisbury/semv"
	"github.com/stretchr/testify/assert"
)

type (
	TestRectClient struct {
		created  []dummyRequest
		deployed []dummyDeploy
		scaled   []dummyScale
	}

	dummyDeploy struct {
		cluster   string
		depID     string
		reqID     string
		imageName string
		res       Resources
	}

	dummyRequest struct {
		cluster string
		id      string
		count   int
	}

	dummyScale struct {
		cluster, reqid string
		count          int
		message        string
	}
)

func NewTRC() TestRectClient {
	return TestRectClient{
		created: []dummyRequest{},
	}
}

func (t *TestRectClient) Deploy(cluster string, depID string, reqID string, imageName string, res Resources) error {
	t.deployed = append(t.deployed, dummyDeploy{cluster, depID, reqID, imageName, res})
	return nil
}

// PostRequest(cluster, request id, instance count)
func (t *TestRectClient) PostRequest(cluster string, id string, count int) error {
	t.created = append(t.created, dummyRequest{cluster, id, count})
	return nil
}

//Scale(cluster url, request id, instance count, message)
func (t *TestRectClient) Scale(cluster, reqid string, count int, message string) error {
	t.scaled = append(t.scaled, dummyScale{cluster, reqid, count, message})
	return nil
}

//ImageName finds or guesses a docker image name for a Deployment
func (t *TestRectClient) ImageName(d *Deployment) (string, error) {
	return d.SourceVersion.String(), nil
}

/* TESTS BEGIN */

func TestModifyScale(t *testing.T) {
	log.SetFlags(log.Flags() | log.Lshortfile)
	assert := assert.New(t)
	pair := &DeploymentPair{
		prior: &Deployment{
			SourceVersion: SourceVersion{
				RepoURL: RepoURL("reqid"),
			},
			DeployConfig: DeployConfig{
				NumInstances: 12,
			},
			Cluster: "cluster",
		},
		post: &Deployment{
			SourceVersion: SourceVersion{
				RepoURL: RepoURL("reqid"),
			},
			DeployConfig: DeployConfig{
				NumInstances: 24,
			},
			Cluster: "cluster",
		},
	}

	chanset := NewDiffChans(1)
	client := TestRectClient{}

	errs := make(chan RectificationError)
	Rectify(chanset, errs, &client)
	chanset.Modified <- pair
	chanset.Close()
	for e := range errs {
		t.Error(e)
	}

	assert.Len(client.deployed, 0)
	assert.Len(client.created, 0)

	if assert.Len(client.scaled, 1) {
		assert.Equal(24, client.scaled[0].count)
	}
}

func TestModifyImage(t *testing.T) {
	assert := assert.New(t)
	before, _ := semv.Parse("1.2.3-test")
	after, _ := semv.Parse("2.3.4-new")
	pair := &DeploymentPair{
		prior: &Deployment{
			SourceVersion: SourceVersion{
				RepoURL: RepoURL("reqid"),
				Version: before,
			},
			DeployConfig: DeployConfig{
				NumInstances: 1,
			},
			Cluster: "cluster",
		},
		post: &Deployment{
			SourceVersion: SourceVersion{
				RepoURL: RepoURL("reqid"),
				Version: after,
			},
			DeployConfig: DeployConfig{
				NumInstances: 1,
			},
			Cluster: "cluster",
		},
	}

	chanset := NewDiffChans(1)
	client := TestRectClient{}

	errs := make(chan RectificationError)
	Rectify(chanset, errs, &client)
	chanset.Modified <- pair
	chanset.Close()
	for e := range errs {
		t.Error(e)
	}

	assert.Len(client.created, 0)
	assert.Len(client.scaled, 0)

	if assert.Len(client.deployed, 1) {
		assert.Regexp("2.3.4", client.deployed[0].imageName)
	}
}

func TestModify(t *testing.T) {
	assert := assert.New(t)
	before, _ := semv.Parse("1.2.3-test")
	after, _ := semv.Parse("2.3.4-new")
	pair := &DeploymentPair{
		prior: &Deployment{
			SourceVersion: SourceVersion{
				RepoURL: RepoURL("reqid"),
				Version: before,
			},
			DeployConfig: DeployConfig{
				NumInstances: 1,
			},
			Cluster: "cluster",
		},
		post: &Deployment{
			SourceVersion: SourceVersion{
				RepoURL: RepoURL("reqid"),
				Version: after,
			},
			DeployConfig: DeployConfig{
				NumInstances: 24,
			},
			Cluster: "cluster",
		},
	}

	chanset := NewDiffChans(1)
	client := TestRectClient{}

	errs := make(chan RectificationError)
	Rectify(chanset, errs, &client)
	chanset.Modified <- pair
	chanset.Close()
	for e := range errs {
		t.Error(e)
	}

	assert.Len(client.created, 0)

	if assert.Len(client.deployed, 1) {
		assert.Regexp("2.3.4", client.deployed[0].imageName)
	}

	if assert.Len(client.scaled, 1) {
		assert.Equal(24, client.scaled[0].count)
	}
}

func TestDeletes(t *testing.T) {
	assert := assert.New(t)

	deleted := &Deployment{
		SourceVersion: SourceVersion{
			RepoURL: RepoURL("reqid"),
		},
		DeployConfig: DeployConfig{
			NumInstances: 12,
		},
		Cluster: "cluster",
	}

	chanset := NewDiffChans(1)
	client := TestRectClient{}

	errs := make(chan RectificationError)
	Rectify(chanset, errs, &client)
	chanset.Deleted <- deleted
	chanset.Close()
	for e := range errs {
		t.Error(e)
	}

	assert.Len(client.deployed, 0)
	assert.Len(client.created, 0)

	if assert.Len(client.scaled, 1) {
		req := client.scaled[0]
		assert.Equal("cluster", req.cluster)
		assert.Equal("reqid", req.reqid)
		assert.Equal(0, req.count)
	}
}

func TestCreates(t *testing.T) {
	assert := assert.New(t)

	chanset := NewDiffChans(1)
	client := TestRectClient{}

	errs := make(chan RectificationError)
	Rectify(chanset, errs, &client)

	created := &Deployment{
		SourceVersion: SourceVersion{
			RepoURL: RepoURL("reqid"),
		},
		DeployConfig: DeployConfig{
			NumInstances: 12,
		},
		Cluster: "cluster",
	}

	chanset.Created <- created

	chanset.Close()

	for e := range errs {
		t.Error(e)
	}

	assert.Len(client.scaled, 0)
	if assert.Len(client.deployed, 1) {
		dep := client.deployed[0]
		assert.Equal("cluster", dep.cluster)
		assert.Equal("reqid 0.0.0", dep.imageName)
	}

	if assert.Len(client.created, 1) {
		req := client.created[0]
		assert.Equal("cluster", req.cluster)
		assert.Equal("reqid", req.id)
		assert.Equal(12, req.count)
	}
}
