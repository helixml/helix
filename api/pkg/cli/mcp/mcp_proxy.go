package mcp

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(runProxyCmd)
}

var runProxyCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Helix mpc (model context protocol) proxy",
	Long:  `TODO`,
	Run: func(*cobra.Command, []string) {
		helixAPIKey := os.Getenv("HELIX_API_KEY")
		helixURL := os.Getenv("HELIX_URL")

		if helixAPIKey == "" || helixURL == "" {
			log.Fatal("HELIX_API_KEY and HELIX_URL must be set")
		}

		u, err := url.Parse(helixURL)
		if err != nil {
			log.Fatal("HELIX_URL must be a valid URL")
		}

		proxy := httputil.NewSingleHostReverseProxy(u)

		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		//
	},
}
