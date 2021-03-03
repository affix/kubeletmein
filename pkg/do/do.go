package do

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/4armed/kubeletmein/pkg/common"
	"github.com/4armed/kubeletmein/pkg/config"
	metadata "github.com/digitalocean/go-metadata"
	"github.com/kubicorn/kubicorn/pkg/logger"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	metadataIP = "169.254.169.254"
)

// Metadata stores the Kubernetes-related YAML
type Metadata struct {
	CaCert       string `yaml:"k8saas_ca_cert"`
	KubeletToken string `yaml:"k8saas_bootstrap_token"`
	KubeMaster   string `yaml:"k8saas_master_domain_name"`
}

// BootstrapCmd represents the bootstrap command
func BootstrapCmd(c *config.Config) *cobra.Command {
	metadataClient := metadata.NewClient()
	m := Metadata{}
	userData := []byte{}
	var kubeMaster string
	var err error

	cmd := &cobra.Command{
		Use:              "do",
		TraverseChildren: true,
		Short:            "Write out a bootstrap kubeconfig for the kubelet LoadClientCert function on Digital Ocean",
		RunE: func(cmd *cobra.Command, args []string) error {

			if c.MetadataFile == "" {
				userData, err = fetchMetadataFromDOService(metadataClient)
				if err != nil {
					return err
				}
			} else {
				logger.Info("fetching kubelet creds from file: %v", c.MetadataFile)
				userData, err = common.FetchMetadataFromFile(c.MetadataFile)
				if err != nil {
					return err
				}
			}

			err = yaml.Unmarshal([]byte(userData), &m)
			if err != nil {
				return fmt.Errorf("unable to parse YAML from user-data: %v", err)
			}

			logger.Info("writing ca cert to: %v", c.CaCertPath)
			err = ioutil.WriteFile(c.CaCertPath, []byte(m.CaCert), 0644)
			if err != nil {
				return fmt.Errorf("unable to write ca cert to file: %v", err)
			}

			if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS") != "" {
				kubeMaster = os.Getenv("KUBERNETES_SERVICE_HOST") + ":" + os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
			} else {
				kubeMaster = m.KubeMaster
			}

			logger.Info("generating bootstrap-kubeconfig file at: %v", c.BootstrapConfig)
			kubeconfigData := clientcmdapi.Config{
				// Define a cluster stanza
				Clusters: map[string]*clientcmdapi.Cluster{"local": {
					Server:                "https://" + kubeMaster,
					InsecureSkipTLSVerify: false,
					CertificateAuthority:  c.CaCertPath,
				}},
				// Define auth based on the kubelet client cert retrieved
				AuthInfos: map[string]*clientcmdapi.AuthInfo{"kubelet": {
					Token: m.KubeletToken,
				}},
				// Define a context and set as current
				Contexts: map[string]*clientcmdapi.Context{"service-account-context": {
					Cluster:  "local",
					AuthInfo: "kubelet",
				}},
				CurrentContext: "service-account-context",
			}

			// Marshal to disk
			err = clientcmd.WriteToFile(kubeconfigData, c.BootstrapConfig)
			if err != nil {
				return fmt.Errorf("unable to write bootstrap-kubeconfig file: %v", err)
			}

			logger.Info("wrote bootstrap-kubeconfig")
			logger.Info("now generate a new node certificate with: kubeletmein generate")

			return err
		},
	}

	return cmd
}

func fetchMetadataFromDOService(metadataClient *metadata.Client) ([]byte, error) {
	logger.Info("fetching kubelet creds from metadata service")

	userData, err := metadataClient.UserData()
	if err != nil {
		return nil, err
	}

	return []byte(userData), nil
}