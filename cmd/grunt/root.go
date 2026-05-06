package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "grunt",
	Short: "Grunt - A simple chat protocol for Grugs",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}