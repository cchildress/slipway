package main

import (
  "encoding/base64"
  "strings"
  "testing"
)

var test_config = config_s{
  ami_id:           "ami-4e79ed36",
  disk_size:        8,
  duration:         0.1,
  instance_size:    "t2.micro",
  launch_count:     1,
  ssh_pub_key_path: "~/.ssh/id_rsa.pub",
  sec_group:        "", // Fill this in with the default from slipway.go
  subnet_id:        "", // Fill this in with the default from slipway.go
}

const expected_provisioner_script = `#!/bin/bash
HELM_SRC="https://storage.googleapis.com/kubernetes-helm/helm-v2.9.1-linux-amd64.tar.gz"
MINIKUBE_SRC="https://github.com/kubernetes/minikube/releases/download/v0.28.0/minikube_0.28-0.deb"
PACKAGES_LIST="build-essential docker.io htop kubectl npm python socat tmux"
SSH_PUB_KEY="asdf
"

echo "Setting DNS name in /etc/hosts"
echo "$(ip route get 1 | awk '{print $NF;exit}') ${HOSTNAME}" >> /etc/hosts

echo "Installing ssh public key"
mkdir -p /home/ubuntu/.ssh
echo ${SSH_PUB_KEY} > /home/ubuntu/.ssh/authorized_keys
chown ubuntu:ubuntu /home/ubuntu/.ssh/authorized_keys

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
usermod -aG docker ubuntu
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

mkdir -p /home/ubuntu/.kube
touch /home/ubuntu/.kube/config
export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export MINIKUBE_HOME=/home/ubuntu
export CHANGE_MINIKUBE_NONE_USER=true
export KUBECONFIG=/home/ubuntu/.kube/config
minikube start --vm-driver=none --kubernetes-version v1.9.3
chown -R ubuntu:ubuntu /home/ubuntu
touch /node_ready
exit 0
END

echo "Install helm"
wget ${HELM_SRC}
tar xzf helm*.tar.gz
cp linux-amd64/helm /usr/local/bin/helm

echo "Rebooting before first use"
reboot
` // END - expected_provisioner_script

// TODO
func TestFunc_allocate_public_ip(t *testing.T) {
  t.Log("Testing allocate_public_ip()")
}

func TestFunc_arg_handler(t *testing.T) {
  t.Log("Testing arg_handler()")
  err := arg_handler(&test_config)
  if err != nil {
    t.Errorf("Err: %v\n", err)
  }
}

func TestFunc_describe_ami(t *testing.T) {
  t.Log("Testing describe_ami()")
  name, root_dev, err := describe_ami("ami-4e79ed36")
  if err != nil {
    t.Errorf("Err: %v\n", err)
  }
  if name != "ubuntu-xenial-16.04-amd64-server-20180306" {
    t.Errorf("AMI name does not match: %v\n", name)
  }
  if root_dev != "/dev/sda1" {
    t.Errorf("Root device does not match: %v\n", root_dev)
  }
}

func TestFunc_get_provisioner(t *testing.T) {
  t.Log("Testing get_provisioner()")
  provisioner_enc, err := get_provisioner("./test_ssh_key")
  if err != nil {
    t.Errorf("Err: %v\n", err)
  }
  provisioner_dec, err := base64.StdEncoding.DecodeString(provisioner_enc)
  if err != nil {
    t.Errorf("Unable to decode provisioner script: %v\n", err)
  }
  provisioner_formatted := strings.Replace(string(provisioner_dec), "\\n", "\n", -1)
  expected_enc := base64.StdEncoding.EncodeToString([]byte(expected_provisioner_script))
  if provisioner_enc != expected_enc {
    t.Errorf("Provisioner script does not match")
    t.Logf("Expected:\n%v", expected_provisioner_script)
    t.Logf("Got:\n%v", provisioner_formatted)
  }
}

// TODO
func TestFunc_make_instances(t *testing.T) {
  t.Log("Testing make_instances()")
}

func TestFunc_prepare_tags(t *testing.T) {
  t.Log("Testing prepare_tags()")
  tags, _ := prepare_tags(&test_config)
  for _, tag := range tags {
    if *tag.Value == "" {
      t.Errorf("Failed to populate tag %v\n", *tag.Key)
    }
  }
}
