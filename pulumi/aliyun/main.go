package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi-alicloud/sdk/v3/go/alicloud"
	"github.com/pulumi/pulumi-alicloud/sdk/v3/go/alicloud/cs"
	"github.com/pulumi/pulumi-alicloud/sdk/v3/go/alicloud/ecs"
	"github.com/pulumi/pulumi-alicloud/sdk/v3/go/alicloud/vpc"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"k8s.io/klog/v2"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		conf := config.New(ctx, "koderover")
		stack := ctx.Stack()

		org := conf.Get("org")
		project := conf.Get("project")

		baseName := fmt.Sprintf("%s-%s-%s", org, project, stack)
		vpcName := baseName
		sgName := fmt.Sprintf("%s-default", baseName)

		vpcCIDR := "10.0.0.0/16"

		rawTags := map[string]string{
			"org":     org,
			"project": project,
			"stack":   stack,
		}
		mapTags := pulumi.Map{}
		stringMapTags := pulumi.StringMap{}
		for k, v := range rawTags {
			mapTags[k] = pulumi.String(v)
			stringMapTags[k] = pulumi.String(v)
		}

		ecsName := baseName
		ecsImageID := "ubuntu_20_04_x64_20G_alibase_20210927.vhd"
		ecsChargeType := "PostPaid"
		ecsType := "ecs.c7.xlarge"
		ecsInternetChargeType := "PayByTraffic"
		ecsInternetMaxBandwidthOut := 5

		// Prepare VPC.
		vpcOutput, err := vpc.NewNetwork(ctx, vpcName, &vpc.NetworkArgs{
			VpcName:   pulumi.StringPtr(vpcName),
			CidrBlock: pulumi.StringPtr(vpcCIDR),
			Tags:      mapTags,
		})
		if err != nil {
			return err
		}

		pulumi.All(vpcOutput.ID(), vpcOutput.VpcName, vpcOutput.Status).ApplyT(func(args []interface{}) string {
			klog.InfoS("VPCInfo", "id", args[0], "name", args[1], "status", args[2])
			return fmt.Sprintf("%s", args[0])
		})

		// Prepare VSwitch.
		zonesOutput, err := alicloud.GetZones(ctx, &alicloud.GetZonesArgs{
			AvailableResourceCreation: strPtr("VSwitch"),
			NetworkType:               strPtr("Vpc"),
		})
		if err != nil {
			return err
		}
		klog.InfoS("ZoneInfo", "zone", strings.Join(zonesOutput.Ids, ","))

		for idx, zoneID := range zonesOutput.Ids {
			zoneSuffixID := strings.Split(zoneID, "-")[2]
			vswitchCIDR := fmt.Sprintf("10.0.%d.0/24", idx)
			vswitchName := fmt.Sprintf("%s-%s", vpcName, zoneSuffixID)
			vswitchOutput, err := vpc.NewSubnet(ctx, vswitchName, &vpc.SubnetArgs{
				VpcId:       vpcOutput.ID(),
				VswitchName: pulumi.StringPtr(vswitchName),
				CidrBlock:   pulumi.String(vswitchCIDR),
				ZoneId:      pulumi.String(zoneID),
				Tags:        mapTags,
			})
			if err != nil {
				return err
			}

			pulumi.All(vswitchOutput.ID(), vswitchOutput.VswitchName, vswitchOutput.Status).ApplyT(func(args []interface{}) string {
				klog.InfoS("VSwitchInfo", "id", args[0], "name", args[1], "status", args[2])

				return fmt.Sprintf("id: %s, name: %s, status: %s", args[0], args[1], args[2])
			})
		}

		// Prepare SecurityGroup.
		sgOutput, err := ecs.NewSecurityGroup(ctx, sgName, &ecs.SecurityGroupArgs{
			Name:              pulumi.StringPtr(sgName),
			VpcId:             vpcOutput.ID(),
			InnerAccessPolicy: pulumi.StringPtr("Accept"),
			SecurityGroupType: pulumi.StringPtr("normal"),
			Tags:              mapTags,
		})
		if err != nil {
			return err
		}
		pulumi.All(sgOutput.ID(), sgOutput.Name).ApplyT(func(args []interface{}) string {
			klog.InfoS("SGInfo", "id", args[0], "name", args[1])

			return fmt.Sprintf("%s", args[0])
		})

		// Set Policy Rules in SecurityGroup.
		sgOutput.ID().ApplyT(func(sgIDI interface{}) error {
			sgID := fmt.Sprintf("%s", sgIDI)

			_, err := ecs.NewSecurityGroupRule(ctx, "icmp", &ecs.SecurityGroupRuleArgs{
				Type:            pulumi.String("ingress"),
				IpProtocol:      pulumi.String("icmp"),
				CidrIp:          pulumi.StringPtr("0.0.0.0/0"),
				SecurityGroupId: pulumi.String(sgID),
			})
			if err != nil {
				klog.ErrorS(err, "SetSecurityGroupPolicy", "policy", "icmp")

				return err
			}

			_, err = ecs.NewSecurityGroupRule(ctx, "tcp-22", &ecs.SecurityGroupRuleArgs{
				Type:            pulumi.String("ingress"),
				IpProtocol:      pulumi.String("tcp"),
				PortRange:       pulumi.StringPtr("22/22"),
				CidrIp:          pulumi.StringPtr("0.0.0.0/0"),
				SecurityGroupId: pulumi.String(sgID),
			})
			if err != nil {
				klog.ErrorS(err, "SetSecurityGroupPolicy", "policy", "tcp-22")

				return err
			}

			_, err = ecs.NewSecurityGroupRule(ctx, "tcp-zadig", &ecs.SecurityGroupRuleArgs{
				Type:            pulumi.String("ingress"),
				IpProtocol:      pulumi.String("tcp"),
				PortRange:       pulumi.StringPtr("30000/30000"),
				CidrIp:          pulumi.StringPtr("0.0.0.0/0"),
				SecurityGroupId: pulumi.String(sgID),
			})
			if err != nil {
				klog.ErrorS(err, "SetSecurityGroupPolicy", "policy", "tcp-zadig")

				return err
			}

			return nil
		})

		// Prepare ASK.
		vswitchIDs := vpcOutput.ID().ApplyT(func(vpcID interface{}) ([]string, error) {
			vpcIDStr := fmt.Sprintf("%s", vpcID)

			vswitchesOutput, err := vpc.GetSwitches(ctx, &vpc.GetSwitchesArgs{
				VpcId: &vpcIDStr,
			})
			if err != nil {
				return nil, err
			}

			klog.InfoS("GetVSwitches", "info", strings.Join(vswitchesOutput.Ids, ","))
			return vswitchesOutput.Ids, nil
		})

		if config.New(ctx, "alicloud").Get("region") == "cn-wulanchabu" {
			askName := baseName
			askVersion := "v1.20.11-aliyun.1"
			askCIDR := "172.16.0.0/24"
			askKubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", fmt.Sprintf("config.ask.%s", stack))
			askSLBSpec := "slb.s1.small"

			askOutput, err := cs.NewServerlessKubernetes(ctx, askName, &cs.ServerlessKubernetesArgs{
				Name:    pulumi.StringPtr(askName),
				Version: pulumi.StringPtr(askVersion),
				VpcId:   vpcOutput.ID(),
				VswitchIds: pulumi.StringArray{
					vswitchIDs.ApplyT(func(arr []string) string {
						return arr[0]
					}).(pulumi.StringOutput),
				},
				ServiceCidr:                 pulumi.String(askCIDR),
				ServiceDiscoveryTypes:       pulumi.ToStringArray([]string{"CoreDNS"}),
				NewNatGateway:               pulumi.BoolPtr(true),
				EndpointPublicAccessEnabled: pulumi.BoolPtr(true),
				LoadBalancerSpec:            pulumi.StringPtr(askSLBSpec),

				KubeConfig:         pulumi.StringPtr(askKubeconfigPath),
				DeletionProtection: pulumi.BoolPtr(false),
				TimeZone:           pulumi.StringPtr(conf.Get("timezone")),
				Tags:               mapTags,
			})
			if err != nil {
				return err
			}

			pulumi.All(askOutput.ID(), askOutput.Name, askOutput.Version).ApplyT(func(args []interface{}) string {
				klog.InfoS("ASKInfo", "id", args[0], "name", args[1], "version", args[2])

				return fmt.Sprintf("id: %s, name: %s, version: %s", args[0], args[1], args[2])
			})
		}

		// Prepare ECS.
		ecsOutput, err := ecs.NewInstance(ctx, ecsName, &ecs.InstanceArgs{
			HostName:        pulumi.StringPtr(ecsName),
			InstanceName:    pulumi.String(ecsName),
			ImageId:         pulumi.String(ecsImageID),
			Password:        conf.GetSecret("zadig-ecs-passwd"),
			AutoRenewPeriod: pulumi.Int(1),

			InstanceType:            pulumi.String(ecsType),
			InstanceChargeType:      pulumi.String(ecsChargeType),
			InternetChargeType:      pulumi.String(ecsInternetChargeType),
			DeletionProtection:      pulumi.Bool(true),
			DryRun:                  pulumi.Bool(false),
			ForceDelete:             pulumi.Bool(false),
			IncludeDataDisks:        pulumi.Bool(true),
			InternetMaxBandwidthOut: pulumi.Int(ecsInternetMaxBandwidthOut),
			PeriodUnit:              pulumi.String("Month"),
			RenewalStatus:           pulumi.String("Normal"),
			SpotStrategy:            pulumi.String("NoSpot"),
			SystemDiskCategory:      pulumi.String("cloud_essd"),
			SystemDiskSize:          pulumi.Int(30),
			VswitchId: vswitchIDs.ApplyT(func(arr []string) string {
				return arr[0]
			}).(pulumi.StringOutput),
			SecurityGroups: pulumi.StringArray{
				sgOutput.ID(),
			},
			Tags: stringMapTags,

			// Value: ["Running", "Stopped"]
			Status: pulumi.StringPtr("Running"),
		})
		if err != nil {
			return err
		}

		pulumi.All(ecsOutput.ID(), ecsOutput.InstanceName, ecsOutput.PublicIp, ecsOutput.Status).ApplyT(func(args []interface{}) string {
			klog.InfoS("ECSInfo", "id", args[0], "name", *(args[1].(*string)), "publicIP", args[2], "status", *(args[3].(*string)))

			return fmt.Sprintf("%s", args[0])
		})

		return nil
	})
}

func strPtr(s string) *string {
	return &s
}
