package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"log"
	"time"
)

// The slipway tags (these carry over from slipway.go)
var instance_search = []*ec2.Filter{
	{
		Name:   aws.String("tag:SlipwayCreated"),
		Values: []*string{aws.String("true")},
	},
	{
		Name:   aws.String("tag-key"),
		Values: []*string{aws.String("ExpiresTimestamp")},
	},
	{
		Name: aws.String("instance-state-name"),
		Values: []*string{
			aws.String("running"),
			aws.String("stopped"),
		},
	},
}

func real_main() {
	expired_nodes := ec2.TerminateInstancesInput{}
	expired_instance_ids := []*string{}
	expired_addresses := []ec2.ReleaseAddressInput{}

	// AWS login / setup
	svc := ec2.New(session.New(&aws.Config{Region: aws.String("us-west-2")}))

	// Get a list of all instances that match the slipway tags.
	result, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: instance_search,
	})
	if err != nil {
		log.Fatalf("Could not get list of instances: %v\n", err)
	}

	// Loop over all instances to look for expired ones.
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			for _, tag := range instance.Tags {
				if *tag.Key == "ExpiresTimestamp" {
					t, err := time.Parse(time.RFC3339, *tag.Value)
					if err != nil {
						log.Printf("Could not parse ExpiresTimestamp tag: %v\n", err)
					}
					if time.Now().After(t) {
						log.Printf("Instance %v has expired (expiration %v).\n", *instance.InstanceId, *tag.Value)
						expired_nodes.InstanceIds = append(expired_nodes.InstanceIds, instance.InstanceId)
						expired_instance_ids = append(expired_instance_ids, aws.String(*instance.InstanceId))
					} else {
						log.Printf("Instance %v is still valid (expiration %v)\n", *instance.InstanceId, *tag.Value)
					}
				}
			}
		}
	}

	// Get a list of associated elastic IPs for the doomed instances.
	result_eip, err := svc.DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-id"),
				Values: expired_instance_ids,
			},
		},
	})
	if err != nil {
		log.Fatalf("Could not get list of EIPs: %v\n", err)
	}

	// Loop over the associated elastic IPs to get the allocation IDs needed to terminate
	for _, address := range result_eip.Addresses {
		expired_addresses = append(expired_addresses, ec2.ReleaseAddressInput{
			AllocationId: address.AllocationId,
		})
	}

	// Terminate the stuff
	if len(expired_nodes.InstanceIds) > 0 {
		// Kill the actual instances
		log.Printf("Terminating instances\n")
		_, err = svc.TerminateInstances(&expired_nodes)
		if err != nil {
			log.Fatalf("Failed terminating instance(s): %v\n", err)
		}

		// Wait until the instances are gone (can't release the IP while it's considered in use)
		log.Printf("Waiting for instances to terminate\n")
		err = svc.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("instance-id"),
					Values: expired_instance_ids,
				},
			},
		})
		if err != nil {
			log.Fatalf("Unable to wait for instance to terminate: %v\n", err)
		}

		// Now release the addresses
		log.Printf("Releasing addresses\n")
		for _, release_s := range expired_addresses {
			log.Printf("Releasing EIP: %v\n", *release_s.AllocationId)
			_, err = svc.ReleaseAddress(&release_s)
			if err != nil {
				log.Fatalf("Failed to release EIP: %v\n", err)
			}
			log.Printf("Released EIP: %v\n", *release_s.AllocationId)
		}
	}

	log.Printf("fin\n")
} // END - real_main()

func main() {
	lambda.Start(real_main)
}
