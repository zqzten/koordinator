package main

import "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/des/provision"

func init() {
	koordinatorPlugins[provision.Name] = provision.New
}
