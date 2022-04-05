package cmd

import (
	"fmt"
	"github.com/sebgl/migrate-elasticsearch/internal"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "migrate-elasticsearch <namespace/name> --from kubecontext-A --to kubecontext-B",
	Short: "Run Elasticsearch between 2 K8s clusters in the same region",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	Run:   run,
}

var (
	fromContext *string
	toContext   *string
)

func init() {
	fromContext = rootCmd.Flags().String("from", "", "Kubectl config context name")
	toContext = rootCmd.Flags().String("to", "", "Kubectl config context name")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		fmt.Println("invalid number of arguments. Expected a single 'namespace/name' argument.")
		os.Exit(1)
	}
	namespace, name, found := strings.Cut(args[0], "/")
	if !found {
		fmt.Printf("invalid argument %s. Expected 'namespace/name'\n", args[0])
		os.Exit(1)
	}
	if namespace == "" || name == "" {
		fmt.Printf("invalid argument %s. Expected 'namespace/name'\n", args[0])
		os.Exit(1)
	}

	if fromContext == nil || *fromContext == "" {
		fmt.Println("--from=<kubeconfig context name> is mandatory")
		os.Exit(1)
	}
	if toContext == nil || *toContext == "" {
		fmt.Println("--to=<kubeconfig context name> is mandatory")
		os.Exit(1)
	}

	config := internal.Config{
		FromContext: *fromContext,
		ToContext:   *toContext,
		Namespace:   namespace,
		Name:        name,
	}
	if err := internal.Run(config); err != nil {
		log.WithError(err).Error("failed to run the migration")
		os.Exit(1)
	}
}
