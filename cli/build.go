package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/lambda"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// TODO: this command is very happy path at the moment

var listBuilds, startBuild bool
var terminateInstanceID, terminateRegion, listName, buildName string

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.AddCommand(buildListCmd)
	buildListCmd.Flags().StringVar(&name, "name", "", "name for stack")

	buildCmd.AddCommand(buildStartCmd)
	buildStartCmd.Flags().StringVar(&name, "name", "", "name for stack")

	buildCmd.AddCommand(buildTerminateCmd)
	buildTerminateCmd.Flags().StringVarP(&terminateInstanceID, "instance-id", "i", "", "EC2 instance id you want to terminate (e.g. i-07ff0f2ed84ff2e8d)")
	buildTerminateCmd.Flags().StringVarP(&terminateRegion, "region", "r", "", "Region of instance you want to terminate")
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Commands to list, start, and terminate builds.",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("Need to specify a subcommand")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {},
}

var buildStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Manually start a build",
	Args: func(cmd *cobra.Command, args []string) error {
		if viper.GetString("name") == "" && name == "" {
			return fmt.Errorf("must provide a stack name")
		}
		if viper.GetString("region") == "" && region == "" {
			return fmt.Errorf("must provide stack region")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if name == "" {
			name = viper.GetString("name")
		}
		if region == "" {
			region = viper.GetString("region")
		}

		sess, err := session.NewSession(aws.NewConfig().WithCredentialsChainVerboseErrors(true))
		if err != nil {
			log.Fatalf("Failed to setup AWS session: %v", err)
		}

		lambdaClient := lambda.New(sess, &aws.Config{Region: &region})
		_, err = lambdaClient.Invoke(&lambda.InvokeInput{
			FunctionName:   aws.String(name + "-build"),
			InvocationType: aws.String("RequestResponse"),
		})
		if err != nil {
			log.Fatalf("Failed to start manual build: %v", err)
		}
		log.Infof("Successfully started manual build for stack %v", name)
	},
}

var buildTerminateCmd = &cobra.Command{
	Use:   "terminate",
	Short: "Terminate a running a build",
	Args: func(cmd *cobra.Command, args []string) error {
		if terminateInstanceID == "" {
			return fmt.Errorf("must provide an instance id to terminate")
		}
		if terminateRegion == "" {
			return fmt.Errorf("must provide region for instance to terminate")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		sess, err := session.NewSession(aws.NewConfig().WithCredentialsChainVerboseErrors(true))
		if err != nil {
			log.Fatalf("Failed to setup AWS session: %v", err)
		}
		ec2Client := ec2.New(sess, &aws.Config{Region: &terminateRegion})
		_, err = ec2Client.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: aws.StringSlice([]string{terminateInstanceID}),
		})
		if err != nil {
			log.Fatalf("Failed to terminate EC2 instance %v in region %v: %v", terminateInstanceID, terminateRegion, err)
		}
		log.Infof("Terminated instance %v in region %v", terminateInstanceID, terminateRegion)
	},
}

var buildListCmd = &cobra.Command{
	Use:   "list",
	Short: "List in progress RattlesnakeOS builds",
	Args: func(cmd *cobra.Command, args []string) error {
		if viper.GetString("name") == "" && name == "" {
			return fmt.Errorf("must provide a stack name")
		}
		if viper.GetString("instance-regions") == "" && instanceRegions == "" {
			return fmt.Errorf("must provide instance regions")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if name == "" {
			name = viper.GetString("name")
		}
		if instanceRegions == "" {
			instanceRegions = viper.GetString("instance-regions")
		}

		sess, err := session.NewSession(aws.NewConfig().WithCredentialsChainVerboseErrors(true))
		if err != nil {
			log.Fatalf("Failed to setup AWS session: %v", err)
		}

		log.Infof("Looking for builds for stack %v in the following regions: %v", name, instanceRegions)
		runningInstances := 0
		for _, region := range strings.Split(instanceRegions, ",") {
			ec2Client := ec2.New(sess, &aws.Config{Region: &region})
			resp, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
				Filters: []*ec2.Filter{
					&ec2.Filter{
						Name:   aws.String("instance-state-name"),
						Values: []*string{aws.String("running")}},
				}})
			if err != nil {
				log.Fatalf("Failed to describe EC2 instances in region %v", region)
			}
			if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
				continue
			}

			for _, reservation := range resp.Reservations {
				for _, instance := range reservation.Instances {
					instanceIamProfileName := strings.Split(*instance.IamInstanceProfile.Arn, "/")[1]
					if instanceIamProfileName == name+"-ec2" {
						log.Printf("Instance '%v': ip='%v' region='%v' launched='%v'", *instance.InstanceId, *instance.PublicIpAddress, region, *instance.LaunchTime)
						runningInstances++
					}
				}
			}
		}
		if runningInstances == 0 {
			log.Info("No active builds found")
		}
	},
}
