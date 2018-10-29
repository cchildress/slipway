package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"io/ioutil"
	"log"
	"os/user"
	"strings"
	"text/template"
	"time"
)

// AWS defaults
const default_ami = "ami-4e79ed36" // Ubuntu 16.04
const default_count = 1
const default_disk_size = 16
const default_duration = 8
const default_sg = "" // Fill me in with something useful
const default_size = "r4.large"
const default_subnet = "" // Fill me in with something useful

// Provisioner script defaults
const ami_user = "ubuntu"
const helm_src = "https://storage.googleapis.com/kubernetes-helm/helm-v2.9.1-linux-amd64.tar.gz"
const kube_ver = "v1.9.3" // Which version of Kubernetes to launch with minikube
const minikube_src = "https://github.com/kubernetes/minikube/releases/download/v0.28.0/minikube_0.28-0.deb"
const packages_list = "build-essential docker.io htop kubectl npm python socat tmux"

const provisioner_script = `#!/bin/bash
HELM_SRC="{{.Helm_src}}"
MINIKUBE_SRC="{{.Minikube_src}}"
PACKAGES_LIST="{{.Packages_list}}"
SSH_PUB_KEY="{{.Ssh_pub_key}}"

echo "Setting DNS name in /etc/hosts"
echo "$(ip route get 1 | awk '{print $NF;exit}') ${HOSTNAME}" >> /etc/hosts

echo "Installing ssh public key"
mkdir -p /home/{{.Ami_user}}/.ssh
echo ${SSH_PUB_KEY} > /home/{{.Ami_user}}/.ssh/authorized_keys
chown {{.Ami_user}}:{{.Ami_user}} /home/{{.Ami_user}}/.ssh/authorized_keys

echo "Installing Kubernetes repo"
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list
echo "Updating and installing packages"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get -y -o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confnew' dist-upgrade
apt-get autoremove -y
apt-get install -y ${PACKAGES_LIST}
rm -rf /var/lib/apt/lists/*
echo "Set permissions and enable docker"
usermod -aG docker {{.Ami_user}}
systemctl enable docker

echo "Install minikube"
wget ${MINIKUBE_SRC}
dpkg -i minikube*.deb

cat <<END > /etc/rc.local
#!/bin/sh -e
#
# rc.local
#
# This script is executed at the end of each multiuser runlevel.
# Make sure that the script will "exit 0" on success or any other
# value on error.
#
# In order to enable or disable this script just change the execution
# bits.
#
# By default this script does nothing.

mkdir -p /home/{{.Ami_user}}/.kube
touch /home/{{.Ami_user}}/.kube/config
export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export MINIKUBE_HOME=/home/{{.Ami_user}}
export CHANGE_MINIKUBE_NONE_USER=true
export KUBECONFIG=/home/{{.Ami_user}}/.kube/config
minikube start --vm-driver=none --kubernetes-version {{.Kube_ver}}
chown -R {{.Ami_user}}:{{.Ami_user}} /home/{{.Ami_user}}
touch /node_ready
exit 0
END

echo "Install helm"
wget ${HELM_SRC}
tar xzf helm*.tar.gz
cp linux-amd64/helm /usr/local/bin/helm

echo "Rebooting before first use"
reboot
` // END - provisioner_script

type config_s struct {
	ami_id           string
	disk_size        int64
	duration         float64
	instance_size    string
	launch_count     int64
	ssh_pub_key_path string
	sec_group        string
	subnet_id        string
}

type provisioner_values struct {
	Ami_user      string
	Helm_src      string
	Kube_ver      string
	Minikube_src  string
	Packages_list string
	Ssh_pub_key   string
}

func allocate_public_ip(config *config_s, instance_id string) (string, error) {
	tags, _ := prepare_tags(config)
	svc := ec2.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))

	result, err := svc.AllocateAddress(&ec2.AllocateAddressInput{})
	if err != nil {
		return "?.?.?.?", fmt.Errorf("Unable to allocate new Elastic IP for instance %v: %v", instance_id, err)
	}
	allocation_id := *result.AllocationId
	elastic_ip := *result.PublicIp

	err = svc.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{&instance_id},
	})
	if err != nil {
		return elastic_ip, fmt.Errorf("Unable to wait for instance provisioning for instance %v: %v", instance_id, err)
	}

	_, err = svc.AssociateAddress(&ec2.AssociateAddressInput{
		AllocationId: &allocation_id,
		InstanceId:   &instance_id,
	})
	if err != nil {
		return elastic_ip, fmt.Errorf("Unable to associate an Elastic IP with instance %v: %v", instance_id, err)
	}

	_, err = svc.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{&allocation_id},
		Tags:      tags,
	})
	if err != nil {
		return elastic_ip, fmt.Errorf("Unable to tag Elastic IP for instance %v: %v", instance_id, err)
	}

	return elastic_ip, nil
} // END - allocate_public_ip()

// Set values in the main config struct on startup
func arg_handler(config *config_s) error {
	flag.StringVar(&config.ami_id, "ami", default_ami, "AMI ID to use")
	flag.Int64Var(&config.launch_count, "count", default_count, "Number of instances to launch")
	flag.Float64Var(&config.duration, "duration", default_duration, "Number of hours to allow the instance to run")
	flag.StringVar(&config.ssh_pub_key_path, "ssh-key", "~/.ssh/id_rsa.pub", "Path to your ssh public key")
	flag.StringVar(&config.instance_size, "size", default_size, "Instance size")
	flag.StringVar(&config.sec_group, "security-group", default_sg, "Security group to attach to the instance")
	flag.Int64Var(&config.disk_size, "storage", default_disk_size, "Size of the root file system in GB")
	flag.StringVar(&config.subnet_id, "subnet", default_subnet, "Subnet ID")
	flag.Parse()

	// Only bother to look up the user's home dir if a key path wasn't set manually
	// We have to do the full lookup because the ~/ isn't parsed here
	if config.ssh_pub_key_path == "~/.ssh/id_rsa.pub" {
		user_s, err := user.Current()
		if err != nil {
			log.Fatalf("Failed to determine current user's home directory: %v\n", err)
		}
		config.ssh_pub_key_path = fmt.Sprintf("%v/.ssh/id_rsa.pub", user_s.HomeDir)
	}

	return nil
} // END - arg_handler()

// Returns the more human readable name for a given AMI ID string
func describe_ami(ami_id string) (string, string, error) {
	svc := ec2.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))
	ami_output, err := svc.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{&ami_id},
	})
	if err != nil {
		return ami_id, "/dev/sda1", err
	}

	if len(ami_output.Images) != 1 {
		return ami_id, "/dev/sda1", fmt.Errorf("AMI search returned %v results\n", ami_output.Images)
	}
	names_split := strings.Split(*ami_output.Images[0].Name, "/")
	return names_split[len(names_split)-1], *ami_output.Images[0].RootDeviceName, nil
} // END - describe_ami()

func get_provisioner(ssh_pub_key_path string) (string, error) {
	ssh_pub_key, err := ioutil.ReadFile(ssh_pub_key_path)
	if err != nil {
		log.Fatalf("Unable to read ssh public key: %v\n", err)
	}

	tmpl, err := template.New("provisioner_script").Parse(provisioner_script)
	if err != nil {
		log.Fatalf("Unable to parse provisioner template: %v\n", err)
	}
	buf := &bytes.Buffer{}
	values := provisioner_values{
		Ami_user:      ami_user,
		Helm_src:      helm_src,
		Kube_ver:      kube_ver,
		Minikube_src:  minikube_src,
		Packages_list: packages_list,
		Ssh_pub_key:   string(ssh_pub_key),
	}
	err = tmpl.Execute(buf, values)
	if err != nil {
		log.Fatalf("Unable to insert values into provisioner template: %v\n", err)
	}

	provisioner_enc := base64.StdEncoding.EncodeToString(buf.Bytes())
	return provisioner_enc, nil
} // END - get_provisioner()

func make_instances(config *config_s, root_dev string, user_data string) {
	tags, expires_time := prepare_tags(config)
	tags_spec := ec2.TagSpecification{
		ResourceType: aws.String("instance"),
		Tags:         tags,
	}

	svc := ec2.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))
	result, err := svc.RunInstances(&ec2.RunInstancesInput{
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			{
				DeviceName: aws.String(root_dev),
				Ebs: &ec2.EbsBlockDevice{
					VolumeSize: aws.Int64(config.disk_size),
				},
			},
		},
		ImageId: aws.String(config.ami_id),
		InstanceInitiatedShutdownBehavior: aws.String("terminate"),
		InstanceType:                      aws.String(config.instance_size),
		MaxCount:                          aws.Int64(config.launch_count),
		MinCount:                          aws.Int64(config.launch_count),
		SecurityGroupIds:                  []*string{&config.sec_group},
		SubnetId:                          aws.String(config.subnet_id),
		UserData:                          aws.String(user_data),
		TagSpecifications:                 []*ec2.TagSpecification{&tags_spec},
	})
	if err != nil {
		log.Println("Could not create instance", err)
		return
	}

	for _, instance := range result.Instances {
		instance_pub_ip, err := allocate_public_ip(config, *instance.InstanceId)
		log.Printf("Created instance %v with public IP %v\n", *instance.InstanceId, instance_pub_ip)
		if err != nil {
			log.Println("    ", err)
		}
	}
	log.Printf("Your instance(s) will expire at %v\n", expires_time.Format(time.RFC850))
} // END - make_instance()

func prepare_tags(config *config_s) ([]*ec2.Tag, time.Time) {
	current_time := time.Now()
	duration_minutes := 60 * config.duration
	expires_time := current_time.Add(time.Minute * time.Duration(duration_minutes))
	user_s, err := user.Current()
	// If we can't deterime the user's name we just default to a John Doe name
	if err != nil {
		log.Printf("Failed to determine current username - using default value 'jdoe'\n")
		user_s := new(user.User)
		user_s.Username = "jdoe"
	}
	name_tag := fmt.Sprintf("%v_local_dev-%v%02v%v", user_s.Username,
		current_time.Year(), int(current_time.Month()), current_time.Day())
	timestamp_tag := current_time.Format(time.RFC3339)
	expires_tag := expires_time.Format(time.RFC3339)

	tags := []*ec2.Tag{
		{
			Key:   aws.String("CreatedTimestamp"),
			Value: &timestamp_tag,
		},
		{
			Key:   aws.String("ExpiresTimestamp"),
			Value: &expires_tag,
		},
		{
			Key:   aws.String("Name"),
			Value: &name_tag,
		},
		{
			Key:   aws.String("SlipwayCreated"),
			Value: aws.String("true"),
		},
		{
			Key:   aws.String("SlipwayUser"),
			Value: &user_s.Username,
		},
	}

	// I return the expiration time object too because some logging needs to reference it.
	return tags, expires_time
}

func main() {
	config := new(config_s)
	err := arg_handler(config)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	log.Printf("Welcome to Slipway.\n")
	ami_name_str, ami_root_dev, err := describe_ami(config.ami_id)
	if err != nil {
		log.Printf("Unable to determine AMI friendly name. (%v)\n", err)
		log.Printf("There could be a problem creating the instance if the root device is not /dev/sda1.\n")
	}
	log.Printf("Creating %v %v instance(s) running %v\n", config.launch_count, config.instance_size, ami_name_str)

	provision_enc, err := get_provisioner(config.ssh_pub_key_path)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	make_instances(config, ami_root_dev, provision_enc)
} // END - main()
