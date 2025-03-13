package webserver

import (
	"math/rand"
	"time"

	"github.com/pulumi/pulumi-openstack/sdk/v4/go/openstack/compute"
	"github.com/pulumi/pulumi-openstack/sdk/v4/go/openstack/networking"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Webserver is a reusable web server component that creates and exports a NIC, public IP, and VM.
type Webserver struct {
	pulumi.ResourceState

	FloatingIP       *networking.FloatingIp
	NetworkInterface *networking.Port
	Instance         *compute.Instance
}

type WebserverArgs struct {
	// A required username for the VM login.
	Username pulumi.StringInput

	// A required encrypted password for the VM password.
	Password pulumi.StringInput

	// An optional boot script that the VM will use.
	BootScript pulumi.StringInput

	// An optional VM size; if unspecified, m1.small will be used.
	VMSize pulumi.StringInput

	// An optional pool from which to allocate the public IP address.
	Pool pulumi.StringInput

	// Required Network ID for the instance
	NetworkID pulumi.StringInput

	// Optional security groups
	SecurityGroups pulumi.StringArrayInput

	// Optional key pair name
	KeyPair pulumi.StringInput
}

// NewWebserver allocates a new web server VM, NIC, and public IP address.
func NewWebserver(ctx *pulumi.Context, name string, args *WebserverArgs, opts ...pulumi.ResourceOption) (*Webserver, error) {
	webserver := &Webserver{}
	err := ctx.RegisterComponentResource("organization:webserver:WebServer", name, webserver, opts...)
	if err != nil {
		return nil, err
	}

	webserver.FloatingIP, err = networking.NewFloatingIp(ctx, name+"-ip", &networking.FloatingIpArgs{
		Pool: args.Pool,
	}, pulumi.Parent(webserver))
	if err != nil {
		return nil, err
	}

	webserver.NetworkInterface, err = networking.NewPort(ctx, name+"-port", &networking.PortArgs{
		NetworkId:      args.NetworkID,
		AdminStateUp:   pulumi.Bool(true),
		SecurityGroups: args.SecurityGroups,
	}, pulumi.Parent(webserver))
	if err != nil {
		return nil, err
	}

	vmSize := args.VMSize
	if vmSize == nil {
		vmSize = pulumi.String("m1.small")
	}

	// Now create the VM, using the resource group and NIC allocated above.
	webserver.Instance, err = compute.NewInstance(ctx, name+"-vm", &compute.InstanceArgs{
		FlavorName:     vmSize,
		ImageName:      pulumi.String("Ubuntu-18.04"),
		KeyPair:        args.KeyPair,
		SecurityGroups: args.SecurityGroups,
		Networks: compute.InstanceNetworkArray{
			compute.InstanceNetworkArgs{
				Uuid:    args.NetworkID,
				Port:    webserver.NetworkInterface.ID(),
				FixedIp: webserver.FloatingIP.FixedIp,
			},
		},
		UserData: args.BootScript,
	}, pulumi.Parent(webserver), pulumi.DependsOn([]pulumi.Resource{webserver.NetworkInterface, webserver.FloatingIP}))
	if err != nil {
		return nil, err
	}

	return webserver, nil
}

func (ws *Webserver) GetIPAddress(ctx *pulumi.Context) pulumi.StringOutput {
	// The public IP address is not allocated until the VM is running, so wait for that resource to create, and then
	// lookup the IP address again to report its public IP.
	ready := pulumi.All(ws.Instance.ID(), ws.FloatingIP.Name, ws.FloatingIP.Pool)
	return ready.ApplyT(func(args []interface{}) (string, error) {
		name := args[1].(string)
		pool := args[2].(string)
		ip, err := networking.GetFloatingIp(ctx, &networking.GetFloatingIpArgs{
			Name: name,
			Pool: pool,
		})
		if err != nil {
			return "", err
		}
		return ip.IpAddress, nil
	}).(pulumi.StringOutput)
}

func rangeIn(low, hi int) int {
	rand.Seed(time.Now().UnixNano())
	return low + rand.Intn(hi-low)
}
