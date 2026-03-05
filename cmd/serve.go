/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start proxy server",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
RunE: func(cmd *cobra.Command, args []string) error {
	portListen := viper.GetInt("lport")
	targetHost := viper.GetString("thost")
	targetPort := viper.GetInt("tport")

	fmt.Printf("Listenting on port: %d\n", portListen)
	fmt.Printf("Redirecting to %s: %d", targetHost, targetPort)
	return nil

},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().Int("lport",8080, "port to run the server on")
	serveCmd.Flags().String("thost","localhost", "target host to forward traffic")
	serveCmd.Flags().Int("tport",8090, "port to forward trafic")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
