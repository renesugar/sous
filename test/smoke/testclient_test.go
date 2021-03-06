//+build smoke

package smoke

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/opentable/sous/config"
	"github.com/opentable/sous/ext/docker"
	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/yaml"
)

type TestClient struct {
	BinPath   string
	ConfigDir string
	// Dir is the working directory.
	Dir string
}

func makeClient(baseDir, sousBin string) TestClient {
	baseDir = path.Join(baseDir, "client1")
	return TestClient{
		BinPath:   sousBin,
		ConfigDir: path.Join(baseDir, "config"),
	}
}

func (c *TestClient) Configure(server, dockerReg string) error {
	if err := os.MkdirAll(c.ConfigDir, 0777); err != nil {
		return err
	}
	conf := config.Config{
		Server: server,
		Docker: docker.Config{
			RegistryHost: dockerReg,
		},
		User: sous.User{
			Name:  "Sous Client1",
			Email: "sous-client1@example.com",
		},
	}
	conf.PollIntervalForClient = 600

	clientDebug := os.Getenv("SOUS_CLIENT_DEBUG") == "true"

	if clientDebug {
		conf.Logging.Basic.Level = "ExtraDebug1"
		conf.Logging.Basic.DisableConsole = false
		conf.Logging.Basic.ExtraConsole = true
	}

	y, err := yaml.Marshal(conf)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(c.ConfigDir, "config.yaml"), y, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func (c *TestClient) Cmd(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := mkCMD(c.Dir, c.BinPath, args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("SOUS_CONFIG_DIR=%s", c.ConfigDir))
	return cmd
}

func (c *TestClient) Run(t *testing.T, args ...string) (string, error) {
	cmd := c.Cmd(t, args...)
	fmt.Fprintf(os.Stderr, "SOUS_CONFIG_DIR = %q\n", c.ConfigDir)
	fmt.Fprintf(os.Stderr, "running sous in %q: %s\n", c.Dir, args)
	// Add quotes to args with spaces for printing.
	for i, a := range args {
		if strings.Contains(a, " ") {
			args[i] = `"` + a + `"`
		}
	}
	out := &bytes.Buffer{}
	cmd.Stdout = io.MultiWriter(os.Stdout, out)
	cmd.Stderr = os.Stderr
	fmt.Fprintf(os.Stderr, "==> sous %s\n", strings.Join(args, " "))
	err := cmd.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return out.String(), err
}

func (c *TestClient) MustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := c.Run(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
