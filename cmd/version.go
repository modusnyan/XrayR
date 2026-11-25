package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version  = "1.2.0"
	codename = "XrayR"
	intro    = "An Xray backend for the Xboard panel"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print current version of XrayR",
		Run: func(cmd *cobra.Command, args []string) {
			showVersion()
		},
	})
}

func showVersion() {
	fmt.Printf("%s %s (%s) \n", codename, version, intro)
}
