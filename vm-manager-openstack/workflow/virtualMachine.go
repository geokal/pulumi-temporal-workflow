package workflow

import (
	"context"
	"fmt"

	"github.com/geokal/pulumi-temporal/vm-manager-openstack/webserver"
	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-random/sdk/v3/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.temporal.io/sdk/activity"
)

func DeployVirtualMachine(ctx context.Context, projectName, vmName string, network *Network) (string, error) {
	logger := activity.GetLogger(ctx)
	stackName := fmt.Sprintf("vmgr%s", vmName)

	logger.Info("Setting up webserver stack " + vmName)

	w, err := auto.NewLocalWorkspace(ctx, auto.Project(workspace.Project{
		Name:    tokens.PackageName(projectName),
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
	}))
	if err != nil {
		return "", errors.Wrap(err, "failed to create workspace")
	}

	err = w.InstallPlugin(ctx, "openstack", "v4.0.0")
	if err != nil {
		return "", errors.Wrap(err, "failed to install program plugins")
	}
	err = w.InstallPlugin(ctx, "random", "v3.1.0")
	if err != nil {
		return "", errors.Wrap(err, "failed to install program plugins")
	}

	user, err := w.WhoAmI(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get authenticated user")
	}

	// create the stack user/vmgr/vmgrXXXXXXXX, could create these stacks under an org instead
	fqsn := auto.FullyQualifiedStackName(user, projectName, stackName)
	s, err := auto.NewStack(ctx, fqsn, w)
	if err != nil {
		return "", errors.Wrap(err, "failed to create stack")
	}

	err = s.SetConfig(ctx, "openstack:authUrl", auto.ConfigValue{Value: "http://your-openstack-url:5000/v3"})
	if err != nil {
		return "", errors.Wrap(err, "failed to set config")
	}
	err = s.SetConfig(ctx, "openstack:region", auto.ConfigValue{Value: "RegionOne"})
	if err != nil {
		return "", errors.Wrap(err, "failed to set config")
	}
	err = s.SetConfig(ctx, "openstack:tenantName", auto.ConfigValue{Value: "your-tenant"})
	if err != nil {
		return "", errors.Wrap(err, "failed to set config")
	}

	// set out program for the deployment with the resulting network info
	w.SetProgram(GetDeployVMFunc(vmName, network))

	logger.Info("Deploying a VM webserver...")

	res, err := s.Up(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to deploy VM stack")
	}

	logger.Info("Deployed a new VM", "ip", res.Outputs["ip"].Value.(string))

	return stackName, nil
}

func GetDeployVMFunc(vmName string, network *Network) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		username := "pulumi"
		password, err := random.NewRandomPassword(ctx, "password", &random.RandomPasswordArgs{
			Length:  pulumi.Int(16),
			Special: pulumi.Bool(false),
		})
		if err != nil {
			return err
		}

		server, err := webserver.NewWebserver(ctx, vmName, &webserver.WebserverArgs{
			Username: pulumi.String(username),
			Password: password.Result,
			BootScript: pulumi.String(fmt.Sprintf(`#!/bin/bash
	echo "Hello, from VMGR!" > index.html
	nohup python -m SimpleHTTPServer 80 &`)),
			NetworkID: pulumi.String(network.NetworkID),
		})
		if err != nil {
			return err
		}

		ctx.Export("ip", server.GetIPAddress(ctx))
		return nil
	}
}
