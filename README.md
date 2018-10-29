# Slipway

![](slipway.gif)

### Slipway
This is a CLI tool that creates temporary EC2 instances for local development work.

You need to have AWS credentials that are valid in `~/.aws/credentials` for this to work.

```
Usage of ./slipway/slipway_darwin64:
  -ami string
    	AMI ID to use (default "ami-4e79ed36")
  -count int
    	Number of instances to launch (default 1)
  -duration float
    	Number of hours to allow the instance to run (default 8)
  -security-group string
    	Security group to attach to the instance
  -size string
    	Instance size (default "r4.large")
  -ssh-key string
    	Path to your ssh public key (default "~/.ssh/id_rsa.pub")
  -storage int
    	Size of the root file system in GB (default 16)
  -subnet string
    	Subnet ID
```

Once the instance is launched you will be given the public IP of the instance. For the default Ubuntu the username is `ubuntu` for ssh.

The provisioning script built into the instance takes some time to run. When the node is completely ready to use a file called `node_ready` will be created at the root of the file system.

To build slipway just run `make` in the slipway directory.

### cull_the_devs
This is a lambda function that should be set up on a cron to handle terminating expired instances.

### TODO
1. Make the lambda function deploy with Serverless.
2. Add a version const to slipway (ideally pull this out of a Makefile var).
3. Maybe make cull_the_devs kill EIPs based on tags instead of their relationship with an instance.
