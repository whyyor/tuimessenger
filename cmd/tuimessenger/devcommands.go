package main

// devCommands are subcommands only some builds have (e.g. demo with -tags devdemo); release builds have none.
var devCommands = map[string]func(args []string) int{}
