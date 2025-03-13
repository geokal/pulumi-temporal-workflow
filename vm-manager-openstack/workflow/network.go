package workflow

import (
	"context"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-openstack/sdk/v4/go/openstack/networking"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.temporal.io/sdk/activity"
)

// Network metadata for VM provisioning
type Network struct {
	NetworkID string
	SubnetID  string
}

// EnsureNetwork creates or finds an existing virtual network where VMs should be placed.
func EnsureNetwork(ctx context.Context, projectName string) (*Network, error) {
	logger := activity.GetLogger(ctx)
	project := workspace.Project{
		Name:    tokens.PackageName(projectName),
		Runtime: workspace.NewProjectRuntimeInfo("go", nil),
	}
	w, err := auto.NewLocalWorkspace(ctx, auto.Program(DeployNetworkFunc), auto.Project(project))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create workspace")
	}

	err = w.InstallPlugin(ctx, "azure", "v3.19.0")
	if err != nil {
		return nil, errors.Wrap(err, "failed to install program plugins")
	}

	user, err := w.WhoAmI(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get authenticated user")
	}

	// create the special networking stack
	fqsn := auto.FullyQualifiedStackName(user, projectName, "networking")
	s, err := auto.NewStack(ctx, fqsn, w)
	if err != nil {
		s, err = auto.SelectStack(ctx, fqsn, w)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create new or select existing network stack")
		}
	}

	outs, err := s.Outputs(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get networking stack outputs")
	}
	resourceGroupName, rgOk := outs["resourceGroupName"].Value.(string)
	subnetID, sidOk := outs["subnetID"].Value.(string)
	if rgOk && sidOk && resourceGroupName != "" && subnetID != "" {
		logger.Info("Found an existing networking stack", "resourceGroupName", resourceGroupName)
		return &Network{resourceGroupName, subnetID}, nil
	}

	err = s.SetConfig(ctx, "azure:location", auto.ConfigValue{Value: "westus"})
	if err != nil {
		return nil, errors.Wrap(err, "failed to set config")
	}

	res, err := s.Up(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to deploy network stack")
	}

	logger.Info("Created a new networking stack", "resourceGroupName", resourceGroupName)
	return &Network{
		res.Outputs["resourceGroupName"].Value.(string),
		res.Outputs["subnetID"].Value.(string)}, nil
}

// DeployNetworkFunc is a pulumi program that sets up a resource group and a virtual network.
func DeployNetworkFunc(ctx *pulumi.Context) error {
	network, err := networking.NewNetwork(ctx, "server-network", &networking.NetworkArgs{
		AdminStateUp: pulumi.Bool(true),
		Name:         pulumi.String("vm-network"),
	})
	if err != nil {
		return err
	}

	subnet, err := networking.NewSubnet(ctx, "server-subnet", &networking.SubnetArgs{
		NetworkId: network.ID(),
		CidrBlock: pulumi.String("192.168.1.0/24"),
		IpVersion: pulumi.Int(4),
	})
	if err != nil {
		return err
	}

	ctx.Export("networkID", network.ID())
	ctx.Export("subnetID", subnet.ID())
	return nil
}
