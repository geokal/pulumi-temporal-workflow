package main

import (
	"log"

	vmworkflow "github.com/geokal/pulumi-temporal/vm-manager-openstack/workflow"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	// The client is a heavyweight object that should be created once
	clientOptions := client.Options{HostPort: "localhost:7233"}
	serviceClient, err := client.NewClient(clientOptions)

	if err != nil {
		log.Fatalf("Unable to create client.  Error: %v", err)
	}

	w := worker.New(serviceClient, "pulumi", worker.Options{})

	w.RegisterWorkflow(vmworkflow.CreateVirtualMachine)
	w.RegisterActivity(vmworkflow.EnsureNetwork)
	w.RegisterActivity(vmworkflow.DeployVirtualMachine)
	w.RegisterActivity(vmworkflow.TearDownVirtualMachine)

	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalf("Unable to start worker.  Error: %v", err)
	}
}
