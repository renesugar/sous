package sous

import (
	"log"
	"regexp"
	"strconv"
	"sync"

	"github.com/opentable/singularity"
	"github.com/opentable/singularity/dtos"
	"github.com/opentable/sous/util/docker_registry"
	"github.com/satori/go.uuid"
)

/*
The imagined use case here is like this:

intendedSet := getFromManifests()
existingSet := getFromSingularity()

dChans := intendedSet.Diff(existingSet)

Rectify(dChans)
*/

type (
	rectifier struct {
		sing RectificationClient
	}

	// RectificationClient abstracts the raw interactions with Singularity.
	// The methods on this interface are tightly bound to the semantics of Singularity itself -
	// it's recommended to interact with the Sous Recify function or the recitification driver
	// rather than with implentations of this interface directly.
	RectificationClient interface {
		// Deploy creates a new deploy on a particular requeust
		Deploy(cluster, depID, reqId, dockerImage string, r Resources) error

		// PostRequest sends a request to a Singularity cluster to initiate
		PostRequest(cluster, reqID string, instanceCount int) error

		//Scale updates the instanceCount associated with a request
		Scale(cluster, reqID string, instanceCount int, message string) error

		//ImageName finds or guesses a docker image name for a Deployment
		ImageName(d *Deployment) string
	}

	RectiAgent struct {
		singClients map[string]singularity.Client
		dockClient  docker_registry.Client
	}

	RectifyComm struct{}
)

func Rectify(dcs DiffChans, s RectificationClient) chan struct{} {
	rect := rectifier{s}
	done := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go rect.rectifyCreates(dcs.Created, wg)
	go rect.rectifyDeletes(dcs.Deleted, wg)
	go rect.rectifyModifys(dcs.Modified, wg)

	go func(r *sync.WaitGroup) {
		r.Wait()
		close(done)
	}(wg)

	return done
}

func (r *rectifier) rectifyCreates(cc chan Deployment, done *sync.WaitGroup) {
	defer done.Done()
	for d := range cc {
		reqID := computeRequestId(&d)
		r.sing.PostRequest(d.Cluster, reqID, d.NumInstances)
		r.sing.Deploy(d.Cluster, newDepID(), reqID, r.computeImageName(&d), d.Resources)
	}
}

func (r *rectifier) rectifyDeletes(dc chan Deployment, done *sync.WaitGroup) {
	defer func(c *sync.WaitGroup) { c.Done() }(done)
	for d := range dc {
		r.sing.Scale(d.Cluster, computeRequestId(&d), 0, "scaling deleted manifest to zero")
	}
}

func (r *rectifier) rectifyModifys(mc chan DeploymentPair, done *sync.WaitGroup) {
	defer func(c *sync.WaitGroup) { c.Done() }(done)
	for pair := range mc {
		if r.changesReq(pair) {
			log.Println("scaling")
			r.sing.Scale(pair.post.Cluster, computeRequestId(pair.post), pair.post.NumInstances, "rectified scaling")
		}

		if changesDep(pair) {
			r.sing.Deploy(pair.post.Cluster, newDepID(), computeRequestId(pair.prior), r.computeImageName(pair.post), pair.post.Resources)
		}
	}
}

func (r *rectifier) computeImageName(d *Deployment) string {
	return r.sing.ImageName(d)
}

func (r rectifier) changesReq(pair DeploymentPair) bool {
	return pair.prior.NumInstances != pair.post.NumInstances
}

func changesDep(pair DeploymentPair) bool {
	return !(pair.prior.SourceVersion.Equal(pair.post.SourceVersion) && pair.prior.Resources.Equal(pair.prior.Resources))
}

func computeRequestId(d *Deployment) string {
	if len(d.RequestId) > 0 {
		return d.RequestId
	}
	return d.CanonicalName().String()
}

var notInIdRE = regexp.MustCompile(`[-/]`)

func idify(in string) string {
	return notInIdRE.ReplaceAllString(in, "")
}

func newDepID() string {
	return idify(uuid.NewV4().String())
}

func (dc *RectifyComm) ImageName(d *Deployment) string {
	// XXX
	// check long-lived cache
	// r.DockerCache

	// query registry based on cache
	// r.DockerClient

	// query registry based on guess
	// enumerate registry
	return d.SourceVersion.DockerImageName()
}

func BuildSingRequest(reqID string, instances int) *dtos.SingularityRequest {
	req := dtos.SingularityRequest{}
	req.LoadMap(map[string]interface{}{
		"Id":          reqID,
		"RequestType": dtos.SingularityRequestRequestTypeSERVICE,
		"Instances":   int32(instances),
	})
	return &req
}

func BuildSingDeployRequest(depID, reqID, imageName string, res Resources) *dtos.SingularityDeployRequest {
	resCpuS, ok := res["cpus"]
	if !ok {
		return nil
	}

	// Ugh. Double blinding of the types for this...
	resCpu, err := strconv.ParseFloat(resCpuS, 64)
	if err != nil {
		return nil
	}

	resMemS, ok := res["memoryMb"]
	if !ok {
		return nil
	}

	resMem, err := strconv.ParseFloat(resMemS, 64)
	if err != nil {
		return nil
	}

	resPortsS, ok := res["numPorts"]
	if !ok {
		return nil
	}

	resPorts, err := strconv.ParseInt(resPortsS, 10, 32)
	if err != nil {
		return nil
	}

	di := dtos.SingularityDockerInfo{}
	di.LoadMap(map[string]interface{}{
		"Image": imageName,
	})

	rez := dtos.Resources{}
	rez.LoadMap(map[string]interface{}{
		"Cpus":     resCpu,
		"MemoryMb": resMem,
		"NumPorts": resPorts,
	})

	ci := dtos.SingularityContainerInfo{}
	ci.LoadMap(map[string]interface{}{
		"Type":   dtos.SingularityContainerInfoSingularityContainerTypeDOCKER,
		"Docker": di,
	})

	dep := dtos.SingularityDeploy{}
	dep.LoadMap(map[string]interface{}{
		"Id":            depID,
		"RequestId":     reqID,
		"Resources":     rez,
		"ContainerInfo": ci,
	})

	dr := dtos.SingularityDeployRequest{}
	dr.LoadMap(map[string]interface{}{
		"Deploy": &dep,
	})

	return &dr
}

func BuildScaleRequest(num int, message string) *dtos.SingularityScaleRequest {
	sr := dtos.SingularityScaleRequest{}
	sr.LoadMap(map[string]interface{}{
		"Id":             newDepID(),
		"Instances":      int32(num),
		"Message":        message,
		"DurationMillis": 60000, // N.b. yo creo this is how long Singularity will allow this attempt to take.
	})
	return &sr
}

/*
package test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"text/template"

	"github.com/opentable/singularity"
	"github.com/opentable/singularity/dtos"
	"github.com/opentable/sous/sous"
	"github.com/opentable/sous/test_with_docker"
	"github.com/opentable/sous/util/docker_registry"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

var ip, registryName, imageName, singularityUrl string

func TestGetLabels(t *testing.T) {
	assert := assert.New(t)
	cl := docker_registry.NewClient()
	cl.BecomeFoolishlyTrusting()

	labels, err := cl.LabelsForImageName(imageName)

	assert.Nil(err)
	assert.Contains(labels, sous.DockerRepoLabel)
}

func TestGetRunningDeploymentSet(t *testing.T) {
	assert := assert.New(t)

	deps, err := sous.GetRunningDeploymentSet([]string{singularityUrl})
	assert.Nil(err)
	assert.Equal(3, len(deps))
	var grafana *sous.Deployment
	for i := range deps {
		if deps[i].SourceVersion.RepoURL == "https://github.com/opentable/docker-grafana.git" {
			grafana = &deps[i]
		}
	}
	if !assert.NotNil(grafana) {
		assert.FailNow("If deployment is nil, other tests will crash")
	}
	assert.Equal(singularityUrl, grafana.Cluster)
	assert.Regexp("^0\\.1", grafana.Resources["cpus"])    // XXX strings and floats...
	assert.Regexp("^100\\.", grafana.Resources["memory"]) // XXX strings and floats...
	assert.Equal("1", grafana.Resources["ports"])         // XXX strings and floats...
	assert.Equal(17, grafana.Patch)
	assert.Equal("91495f1b1630084e301241100ecf2e775f6b672c", grafana.Meta)
	assert.Equal(1, grafana.NumInstances)
	assert.Equal(sous.ManifestKindService, grafana.Kind)
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(wrapCompose(m))
}

func wrapCompose(m *testing.M) (resultCode int) {
	log.SetFlags(log.Flags() | log.Lshortfile)

	if testing.Short() {
		return 0
	}

	defer func() {
		log.Println("Cleaning up...")
		if err := recover(); err != nil {
			log.Print("Panic: ", err)
			resultCode = 1
		}
	}()

	machineName := "default"
	if envMN := os.Getenv("TEST_DOCKER_MACHINE"); len(envMN) > 0 {
		machineName = envMN
	}
	test_agent := test_with_docker.NewAgent(300.0, machineName)

	ip, err := test_agent.IP()
	if err != nil {
		panic(err)
	}

	composeDir := "test-registry"
	registryCertName := "testing.crt"
	registryName = fmt.Sprintf("%s:%d", ip, 5000)

	err = registryCerts(test_agent, composeDir, registryCertName, ip)
	if err != nil {
		panic(fmt.Errorf("building registry certs failed: %s", err))
	}

	started, err := test_agent.ComposeServices(composeDir, map[string]uint{"Singularity": 7099, "Registry": 5000})
	defer test_agent.Shutdown(started)

	registerAndDeploy(ip, "hello-labels", "hello-labels", []int32{})
	registerAndDeploy(ip, "hello-server-labels", "hello-server-labels", []int32{80})
	registerAndDeploy(ip, "grafana-repo", "grafana-labels", []int32{})
	imageName = fmt.Sprintf("%s/%s:%s", registryName, "grafana-repo", "latest")

	log.Println("   *** Beginning tests... ***\n\n")
	resultCode = m.Run()
	return
}

func registerAndDeploy(ip net.IP, reponame, dir string, ports []int32) (err error) {
	imageName := fmt.Sprintf("%s/%s:%s", registryName, reponame, "latest")
	err = buildAndPushContainer(dir, imageName)
	if err != nil {
		panic(fmt.Errorf("building test container failed: %s", err))
	}

	singularityUrl = fmt.Sprintf("http://%s:%d/singularity", ip, 7099)
	err = startInstance(singularityUrl, imageName, ports)
	if err != nil {
		panic(fmt.Errorf("starting a singularity instance failed: %s", err))
	}

	return
}

var successfulBuildRE = regexp.MustCompile(`Successfully built (\w+)`)

type dtoMap map[string]interface{}

func loadMap(fielder dtos.Fielder, m dtoMap) dtos.Fielder {
	_, err := dtos.LoadMap(fielder, m)
	if err != nil {
		log.Fatal(err)
	}

	return fielder
}

func startInstance(url, imageName string, ports []int32) error {
	reqId := idify(imageName)

	sing := singularity.NewClient(url)

	req := loadMap(&dtos.SingularityRequest{}, map[string]interface{}{
		"Id":          reqId,
		"RequestType": dtos.SingularityRequestRequestTypeSERVICE,
		"Instances":   int32(1),
	}).(*dtos.SingularityRequest)

	_, err := sing.PostRequest(req)
	if err != nil {
		return err
	}

	dockerInfo := loadMap(&dtos.SingularityDockerInfo{}, dtoMap{
		"Image": imageName,
	}).(*dtos.SingularityDockerInfo)

	depReq := loadMap(&dtos.SingularityDeployRequest{}, dtoMap{
		"Deploy": loadMap(&dtos.SingularityDeploy{}, dtoMap{
			"Id":        idify(uuid.NewV4().String()),
			"RequestId": reqId,
			"Resources": loadMap(&dtos.Resources{}, dtoMap{
				"Cpus":     0.1,
				"MemoryMb": 100.0,
				"NumPorts": int32(1),
			}),
			"ContainerInfo": loadMap(&dtos.SingularityContainerInfo{}, dtoMap{
				"Type":   dtos.SingularityContainerInfoSingularityContainerTypeDOCKER,
				"Docker": dockerInfo,
			}),
		}),
	}).(*dtos.SingularityDeployRequest)

	_, err = sing.Deploy(depReq)
	if err != nil {
		return err
	}

	return nil
}

func buildAndPushContainer(containerDir, tagName string) error {
	build := exec.Command("docker", "build", ".")
	build.Dir = containerDir
	output, err := build.CombinedOutput()
	if err != nil {
		log.Print("Problem building container: ", containerDir, "\n", string(output))
		return err
	}

	match := successfulBuildRE.FindStringSubmatch(string(output))
	if match == nil {
		return fmt.Errorf("Couldn't find container id in:\n%s", output)
	}

	containerId := match[1]
	tag := exec.Command("docker", "tag", containerId, tagName)
	tag.Dir = containerDir
	output, err = tag.CombinedOutput()
	if err != nil {
		return err
	}

	push := exec.Command("docker", "push", tagName)
	push.Dir = containerDir
	output, err = push.CombinedOutput()
	if err != nil {
		return err
	}

	return nil
}

func registryCerts(test_agent test_with_docker.Agent, composeDir, registryCertName string, ip net.IP) error {
	certPath := filepath.Join(composeDir, registryCertName)
	caPath := fmt.Sprintf("/etc/docker/certs.d/%s/ca.crt", registryName)

	certIPs, err := getCertIPSans(filepath.Join(composeDir, registryCertName))
	if err != nil {
		return err
	}

	haveIP := false

	for _, certIP := range certIPs {
		if certIP.Equal(ip) {
			haveIP = true
			break
		}
	}

	if !haveIP {
		log.Print("Rebuilding the registry certificate")
		certIPs = append(certIPs, ip)
		err = buildTestingKeypair(composeDir, certIPs)
		if err != nil {
			return err
		}

		err = test_agent.RebuildService(composeDir, "registry")
		if err != nil {
			return err
		}
	}

	differs, err := test_agent.DifferingFiles([]string{certPath, caPath})
	if err != nil {
		return err
	}

	for _, diff := range differs {
		local, remote := diff[0], diff[1]
		err = test_agent.InstallFile(local, remote)

		if err != nil {
			return fmt.Errorf("installFile failed: %s", err)
		}
	}

	if len(differs) > 0 {
		err = test_agent.RestartDaemon()
		if err != nil {
			return fmt.Errorf("restarting docker machine's daemon failed: %s", err)
		}
	}
	return err
}

func buildTestingKeypair(dir string, certIPs []net.IP) (err error) {
	log.Print(certIPs)
	selfSignConf := "self-signed.conf"
	temp := template.Must(template.New("req").Parse(`
{{- "" -}}
[ req ]
prompt = no
distinguished_name=req_distinguished_name
x509_extensions = va_c3
encrypt_key = no
default_keyfile=testing.key
default_md = sha256

[ va_c3 ]
basicConstraints=critical,CA:true,pathlen:1
{{range . -}}
subjectAltName = IP:{{.}}
{{end}}
[ req_distinguished_name ]
CN=registry.test
{{"" -}}
		`))
	confPath := filepath.Join(dir, selfSignConf)
	reqFile, err := os.Create(confPath)
	if err != nil {
		return
	}
	err = temp.Execute(reqFile, certIPs)
	if err != nil {
		return
	}

	// This is the openssl command to produce a (very weak) self-signed keypair based on the conf templated above.
	// Ultimately, this provides the bare minimum to use the docker registry on a "remote" server
	openssl := exec.Command("openssl", "req", "-newkey", "rsa:512", "-x509", "-days", "365", "-out", "testing.crt", "-config", selfSignConf, "-batch")
	openssl.Dir = dir
	_, err = openssl.CombinedOutput()

	return
}

func getCertIPSans(certPath string) ([]net.IP, error) {
	certFile, err := os.Open(certPath)
	if _, ok := err.(*os.PathError); ok {
		return make([]net.IP, 0), nil
	}
	if err != nil {
		return nil, err
	}

	certBuf := bytes.Buffer{}
	_, err = certBuf.ReadFrom(certFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(certBuf.Bytes())

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert.IPAddresses, nil
}
*/