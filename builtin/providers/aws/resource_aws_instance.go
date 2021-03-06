package aws

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsInstance() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsInstanceCreate,
		Read:   resourceAwsInstanceRead,
		Update: resourceAwsInstanceUpdate,
		Delete: resourceAwsInstanceDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		SchemaVersion: 1,
		MigrateState:  resourceAwsInstanceMigrateState,

		Schema: map[string]*schema.Schema{
			"ami": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"associate_public_ip_address": {
				Type:     schema.TypeBool,
				ForceNew: true,
				Computed: true,
				Optional: true,
			},

			"availability_zone": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"placement_group": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"instance_type": {
				Type:     schema.TypeString,
				Required: true,
			},

			"key_name": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},

			"subnet_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"private_ip": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Computed: true,
			},

			"source_dest_check": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},

			"user_data": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				StateFunc: func(v interface{}) string {
					switch v.(type) {
					case string:
						return userDataHashSum(v.(string))
					default:
						return ""
					}
				},
			},

			"security_groups": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				ForceNew: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},

			"vpc_security_group_ids": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},

			"public_dns": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"network_interface_id": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"public_ip": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"instance_state": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"private_dns": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"ebs_optimized": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},

			"disable_api_termination": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"instance_initiated_shutdown_behavior": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"monitoring": {
				Type:     schema.TypeBool,
				Optional: true,
			},

			"iam_instance_profile": {
				Type:     schema.TypeString,
				ForceNew: true,
				Optional: true,
			},

			"tenancy": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"tags": tagsSchema(),

			"block_device": {
				Type:     schema.TypeMap,
				Optional: true,
				Removed:  "Split out into three sub-types; see Changelog and Docs",
			},

			"ebs_block_device": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"delete_on_termination": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},

						"device_name": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},

						"encrypted": {
							Type:     schema.TypeBool,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},

						"iops": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},

						"snapshot_id": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},

						"volume_size": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},

						"volume_type": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},
					},
				},
				Set: func(v interface{}) int {
					var buf bytes.Buffer
					m := v.(map[string]interface{})
					buf.WriteString(fmt.Sprintf("%s-", m["device_name"].(string)))
					buf.WriteString(fmt.Sprintf("%s-", m["snapshot_id"].(string)))
					return hashcode.String(buf.String())
				},
			},

			"ephemeral_block_device": {
				Type:     schema.TypeSet,
				Optional: true,
				Computed: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"device_name": {
							Type:     schema.TypeString,
							Required: true,
						},

						"virtual_name": {
							Type:     schema.TypeString,
							Optional: true,
						},

						"no_device": {
							Type:     schema.TypeBool,
							Optional: true,
						},
					},
				},
				Set: func(v interface{}) int {
					var buf bytes.Buffer
					m := v.(map[string]interface{})
					buf.WriteString(fmt.Sprintf("%s-", m["device_name"].(string)))
					buf.WriteString(fmt.Sprintf("%s-", m["virtual_name"].(string)))
					if v, ok := m["no_device"].(bool); ok && v {
						buf.WriteString(fmt.Sprintf("%t-", v))
					}
					return hashcode.String(buf.String())
				},
			},

			"root_block_device": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					// "You can only modify the volume size, volume type, and Delete on
					// Termination flag on the block device mapping entry for the root
					// device volume." - bit.ly/ec2bdmap
					Schema: map[string]*schema.Schema{
						"delete_on_termination": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
							ForceNew: true,
						},

						"iops": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},

						"volume_size": {
							Type:     schema.TypeInt,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},

						"volume_type": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
							ForceNew: true,
						},
					},
				},
			},
		},
	}
}

func resourceAwsInstanceCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	instanceOpts, err := buildAwsInstanceOpts(d, meta)
	if err != nil {
		return err
	}

	// Build the creation struct
	runOpts := &ec2.RunInstancesInput{
		BlockDeviceMappings:   instanceOpts.BlockDeviceMappings,
		DisableApiTermination: instanceOpts.DisableAPITermination,
		EbsOptimized:          instanceOpts.EBSOptimized,
		Monitoring:            instanceOpts.Monitoring,
		IamInstanceProfile:    instanceOpts.IAMInstanceProfile,
		ImageId:               instanceOpts.ImageID,
		InstanceInitiatedShutdownBehavior: instanceOpts.InstanceInitiatedShutdownBehavior,
		InstanceType:                      instanceOpts.InstanceType,
		KeyName:                           instanceOpts.KeyName,
		MaxCount:                          aws.Int64(int64(1)),
		MinCount:                          aws.Int64(int64(1)),
		NetworkInterfaces:                 instanceOpts.NetworkInterfaces,
		Placement:                         instanceOpts.Placement,
		PrivateIpAddress:                  instanceOpts.PrivateIPAddress,
		SecurityGroupIds:                  instanceOpts.SecurityGroupIDs,
		SecurityGroups:                    instanceOpts.SecurityGroups,
		SubnetId:                          instanceOpts.SubnetID,
		UserData:                          instanceOpts.UserData64,
	}

	// Create the instance
	log.Printf("[DEBUG] Run configuration: %s", runOpts)

	var runResp *ec2.Reservation
	err = resource.Retry(15*time.Second, func() *resource.RetryError {
		var err error
		runResp, err = conn.RunInstances(runOpts)
		// IAM instance profiles can take ~10 seconds to propagate in AWS:
		// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html#launch-instance-with-role-console
		if isAWSErr(err, "InvalidParameterValue", "Invalid IAM Instance Profile") {
			log.Print("[DEBUG] Invalid IAM Instance Profile referenced, retrying...")
			return resource.RetryableError(err)
		}
		// IAM roles can also take time to propagate in AWS:
		if isAWSErr(err, "InvalidParameterValue", " has no associated IAM Roles") {
			log.Print("[DEBUG] IAM Instance Profile appears to have no IAM roles, retrying...")
			return resource.RetryableError(err)
		}
		return resource.NonRetryableError(err)
	})
	// Warn if the AWS Error involves group ids, to help identify situation
	// where a user uses group ids in security_groups for the Default VPC.
	//   See https://github.com/hashicorp/terraform/issues/3798
	if isAWSErr(err, "InvalidParameterValue", "groupId is invalid") {
		return fmt.Errorf("Error launching instance, possible mismatch of Security Group IDs and Names. See AWS Instance docs here: %s.\n\n\tAWS Error: %s", "https://terraform.io/docs/providers/aws/r/instance.html", err.(awserr.Error).Message())
	}
	if err != nil {
		return fmt.Errorf("Error launching source instance: %s", err)
	}
	if runResp == nil || len(runResp.Instances) == 0 {
		return errors.New("Error launching source instance: no instances returned in response")
	}

	instance := runResp.Instances[0]
	log.Printf("[INFO] Instance ID: %s", *instance.InstanceId)

	// Store the resulting ID so we can look this up later
	d.SetId(*instance.InstanceId)

	// Wait for the instance to become running so we can get some attributes
	// that aren't available until later.
	log.Printf(
		"[DEBUG] Waiting for instance (%s) to become running",
		*instance.InstanceId)

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"pending"},
		Target:     []string{"running"},
		Refresh:    InstanceStateRefreshFunc(conn, *instance.InstanceId),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	instanceRaw, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(
			"Error waiting for instance (%s) to become ready: %s",
			*instance.InstanceId, err)
	}

	instance = instanceRaw.(*ec2.Instance)

	// Initialize the connection info
	if instance.PublicIpAddress != nil {
		d.SetConnInfo(map[string]string{
			"type": "ssh",
			"host": *instance.PublicIpAddress,
		})
	} else if instance.PrivateIpAddress != nil {
		d.SetConnInfo(map[string]string{
			"type": "ssh",
			"host": *instance.PrivateIpAddress,
		})
	}

	// Update if we need to
	return resourceAwsInstanceUpdate(d, meta)
}

func resourceAwsInstanceRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	resp, err := conn.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(d.Id())},
	})
	if err != nil {
		// If the instance was not found, return nil so that we can show
		// that the instance is gone.
		if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidInstanceID.NotFound" {
			d.SetId("")
			return nil
		}

		// Some other error, report it
		return err
	}

	// If nothing was found, then return no state
	if len(resp.Reservations) == 0 {
		d.SetId("")
		return nil
	}

	instance := resp.Reservations[0].Instances[0]

	if instance.State != nil {
		// If the instance is terminated, then it is gone
		if *instance.State.Name == "terminated" {
			d.SetId("")
			return nil
		}

		d.Set("instance_state", instance.State.Name)
	}

	if instance.Placement != nil {
		d.Set("availability_zone", instance.Placement.AvailabilityZone)
	}
	if instance.Placement.Tenancy != nil {
		d.Set("tenancy", instance.Placement.Tenancy)
	}

	d.Set("ami", instance.ImageId)
	d.Set("instance_type", instance.InstanceType)
	d.Set("key_name", instance.KeyName)
	d.Set("public_dns", instance.PublicDnsName)
	d.Set("public_ip", instance.PublicIpAddress)
	d.Set("private_dns", instance.PrivateDnsName)
	d.Set("private_ip", instance.PrivateIpAddress)
	d.Set("iam_instance_profile", iamInstanceProfileArnToName(instance.IamInstanceProfile))

	if len(instance.NetworkInterfaces) > 0 {
		for _, ni := range instance.NetworkInterfaces {
			if *ni.Attachment.DeviceIndex == 0 {
				d.Set("subnet_id", ni.SubnetId)
				d.Set("network_interface_id", ni.NetworkInterfaceId)
				d.Set("associate_public_ip_address", ni.Association != nil)
			}
		}
	} else {
		d.Set("subnet_id", instance.SubnetId)
		d.Set("network_interface_id", "")
	}
	d.Set("ebs_optimized", instance.EbsOptimized)
	if instance.SubnetId != nil && *instance.SubnetId != "" {
		d.Set("source_dest_check", instance.SourceDestCheck)
	}

	if instance.Monitoring != nil && instance.Monitoring.State != nil {
		monitoringState := *instance.Monitoring.State
		d.Set("monitoring", monitoringState == "enabled" || monitoringState == "pending")
	}

	d.Set("tags", tagsToMap(instance.Tags))

	if err := readSecurityGroups(d, instance); err != nil {
		return err
	}

	if err := readBlockDevices(d, instance, conn); err != nil {
		return err
	}
	if _, ok := d.GetOk("ephemeral_block_device"); !ok {
		d.Set("ephemeral_block_device", []interface{}{})
	}

	// Instance attributes
	{
		attr, err := conn.DescribeInstanceAttribute(&ec2.DescribeInstanceAttributeInput{
			Attribute:  aws.String("disableApiTermination"),
			InstanceId: aws.String(d.Id()),
		})
		if err != nil {
			return err
		}
		d.Set("disable_api_termination", attr.DisableApiTermination.Value)
	}
	{
		attr, err := conn.DescribeInstanceAttribute(&ec2.DescribeInstanceAttributeInput{
			Attribute:  aws.String(ec2.InstanceAttributeNameUserData),
			InstanceId: aws.String(d.Id()),
		})
		if err != nil {
			return err
		}
		if attr.UserData.Value != nil {
			d.Set("user_data", userDataHashSum(*attr.UserData.Value))
		}
	}

	return nil
}

func resourceAwsInstanceUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	d.Partial(true)
	if err := setTags(conn, d); err != nil {
		return err
	} else {
		d.SetPartial("tags")
	}

	if d.HasChange("source_dest_check") || d.IsNewResource() {
		// SourceDestCheck can only be set on VPC instances	// AWS will return an error of InvalidParameterCombination if we attempt
		// to modify the source_dest_check of an instance in EC2 Classic
		log.Printf("[INFO] Modifying `source_dest_check` on Instance %s", d.Id())
		_, err := conn.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
			InstanceId: aws.String(d.Id()),
			SourceDestCheck: &ec2.AttributeBooleanValue{
				Value: aws.Bool(d.Get("source_dest_check").(bool)),
			},
		})
		if err != nil {
			if ec2err, ok := err.(awserr.Error); ok {
				// Toloerate InvalidParameterCombination error in Classic, otherwise
				// return the error
				if "InvalidParameterCombination" != ec2err.Code() {
					return err
				}
				log.Printf("[WARN] Attempted to modify SourceDestCheck on non VPC instance: %s", ec2err.Message())
			}
		}
	}

	if d.HasChange("vpc_security_group_ids") {
		var groups []*string
		if v := d.Get("vpc_security_group_ids").(*schema.Set); v.Len() > 0 {
			for _, v := range v.List() {
				groups = append(groups, aws.String(v.(string)))
			}
		}
		_, err := conn.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
			InstanceId: aws.String(d.Id()),
			Groups:     groups,
		})
		if err != nil {
			return err
		}
	}

	if d.HasChange("instance_type") && !d.IsNewResource() {
		log.Printf("[INFO] Stopping Instance %q for instance_type change", d.Id())
		_, err := conn.StopInstances(&ec2.StopInstancesInput{
			InstanceIds: []*string{aws.String(d.Id())},
		})

		stateConf := &resource.StateChangeConf{
			Pending:    []string{"pending", "running", "shutting-down", "stopped", "stopping"},
			Target:     []string{"stopped"},
			Refresh:    InstanceStateRefreshFunc(conn, d.Id()),
			Timeout:    10 * time.Minute,
			Delay:      10 * time.Second,
			MinTimeout: 3 * time.Second,
		}

		_, err = stateConf.WaitForState()
		if err != nil {
			return fmt.Errorf(
				"Error waiting for instance (%s) to stop: %s", d.Id(), err)
		}

		log.Printf("[INFO] Modifying instance type %s", d.Id())
		_, err = conn.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
			InstanceId: aws.String(d.Id()),
			InstanceType: &ec2.AttributeValue{
				Value: aws.String(d.Get("instance_type").(string)),
			},
		})
		if err != nil {
			return err
		}

		log.Printf("[INFO] Starting Instance %q after instance_type change", d.Id())
		_, err = conn.StartInstances(&ec2.StartInstancesInput{
			InstanceIds: []*string{aws.String(d.Id())},
		})

		stateConf = &resource.StateChangeConf{
			Pending:    []string{"pending", "stopped"},
			Target:     []string{"running"},
			Refresh:    InstanceStateRefreshFunc(conn, d.Id()),
			Timeout:    10 * time.Minute,
			Delay:      10 * time.Second,
			MinTimeout: 3 * time.Second,
		}

		_, err = stateConf.WaitForState()
		if err != nil {
			return fmt.Errorf(
				"Error waiting for instance (%s) to become ready: %s",
				d.Id(), err)
		}
	}

	if d.HasChange("disable_api_termination") {
		_, err := conn.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
			InstanceId: aws.String(d.Id()),
			DisableApiTermination: &ec2.AttributeBooleanValue{
				Value: aws.Bool(d.Get("disable_api_termination").(bool)),
			},
		})
		if err != nil {
			return err
		}
	}

	if d.HasChange("instance_initiated_shutdown_behavior") {
		log.Printf("[INFO] Modifying instance %s", d.Id())
		_, err := conn.ModifyInstanceAttribute(&ec2.ModifyInstanceAttributeInput{
			InstanceId: aws.String(d.Id()),
			InstanceInitiatedShutdownBehavior: &ec2.AttributeValue{
				Value: aws.String(d.Get("instance_initiated_shutdown_behavior").(string)),
			},
		})
		if err != nil {
			return err
		}
	}

	if d.HasChange("monitoring") {
		var mErr error
		if d.Get("monitoring").(bool) {
			log.Printf("[DEBUG] Enabling monitoring for Instance (%s)", d.Id())
			_, mErr = conn.MonitorInstances(&ec2.MonitorInstancesInput{
				InstanceIds: []*string{aws.String(d.Id())},
			})
		} else {
			log.Printf("[DEBUG] Disabling monitoring for Instance (%s)", d.Id())
			_, mErr = conn.UnmonitorInstances(&ec2.UnmonitorInstancesInput{
				InstanceIds: []*string{aws.String(d.Id())},
			})
		}
		if mErr != nil {
			return fmt.Errorf("[WARN] Error updating Instance monitoring: %s", mErr)
		}
	}

	// TODO(mitchellh): wait for the attributes we modified to
	// persist the change...

	d.Partial(false)

	return resourceAwsInstanceRead(d, meta)
}

func resourceAwsInstanceDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	if err := awsTerminateInstance(conn, d.Id()); err != nil {
		return err
	}

	d.SetId("")
	return nil
}

// InstanceStateRefreshFunc returns a resource.StateRefreshFunc that is used to watch
// an EC2 instance.
func InstanceStateRefreshFunc(conn *ec2.EC2, instanceID string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := conn.DescribeInstances(&ec2.DescribeInstancesInput{
			InstanceIds: []*string{aws.String(instanceID)},
		})
		if err != nil {
			if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "InvalidInstanceID.NotFound" {
				// Set this to nil as if we didn't find anything.
				resp = nil
			} else {
				log.Printf("Error on InstanceStateRefresh: %s", err)
				return nil, "", err
			}
		}

		if resp == nil || len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
			// Sometimes AWS just has consistency issues and doesn't see
			// our instance yet. Return an empty state.
			return nil, "", nil
		}

		i := resp.Reservations[0].Instances[0]
		return i, *i.State.Name, nil
	}
}

func readBlockDevices(d *schema.ResourceData, instance *ec2.Instance, conn *ec2.EC2) error {
	ibds, err := readBlockDevicesFromInstance(instance, conn)
	if err != nil {
		return err
	}

	if err := d.Set("ebs_block_device", ibds["ebs"]); err != nil {
		return err
	}

	// This handles the import case which needs to be defaulted to empty
	if _, ok := d.GetOk("root_block_device"); !ok {
		if err := d.Set("root_block_device", []interface{}{}); err != nil {
			return err
		}
	}

	if ibds["root"] != nil {
		roots := []interface{}{ibds["root"]}
		if err := d.Set("root_block_device", roots); err != nil {
			return err
		}
	}

	return nil
}

func readBlockDevicesFromInstance(instance *ec2.Instance, conn *ec2.EC2) (map[string]interface{}, error) {
	blockDevices := make(map[string]interface{})
	blockDevices["ebs"] = make([]map[string]interface{}, 0)
	blockDevices["root"] = nil

	instanceBlockDevices := make(map[string]*ec2.InstanceBlockDeviceMapping)
	for _, bd := range instance.BlockDeviceMappings {
		if bd.Ebs != nil {
			instanceBlockDevices[*bd.Ebs.VolumeId] = bd
		}
	}

	if len(instanceBlockDevices) == 0 {
		return nil, nil
	}

	volIDs := make([]*string, 0, len(instanceBlockDevices))
	for volID := range instanceBlockDevices {
		volIDs = append(volIDs, aws.String(volID))
	}

	// Need to call DescribeVolumes to get volume_size and volume_type for each
	// EBS block device
	volResp, err := conn.DescribeVolumes(&ec2.DescribeVolumesInput{
		VolumeIds: volIDs,
	})
	if err != nil {
		return nil, err
	}

	for _, vol := range volResp.Volumes {
		instanceBd := instanceBlockDevices[*vol.VolumeId]
		bd := make(map[string]interface{})

		if instanceBd.Ebs != nil && instanceBd.Ebs.DeleteOnTermination != nil {
			bd["delete_on_termination"] = *instanceBd.Ebs.DeleteOnTermination
		}
		if vol.Size != nil {
			bd["volume_size"] = *vol.Size
		}
		if vol.VolumeType != nil {
			bd["volume_type"] = *vol.VolumeType
		}
		if vol.Iops != nil {
			bd["iops"] = *vol.Iops
		}

		if blockDeviceIsRoot(instanceBd, instance) {
			blockDevices["root"] = bd
		} else {
			if instanceBd.DeviceName != nil {
				bd["device_name"] = *instanceBd.DeviceName
			}
			if vol.Encrypted != nil {
				bd["encrypted"] = *vol.Encrypted
			}
			if vol.SnapshotId != nil {
				bd["snapshot_id"] = *vol.SnapshotId
			}

			blockDevices["ebs"] = append(blockDevices["ebs"].([]map[string]interface{}), bd)
		}
	}

	return blockDevices, nil
}

func blockDeviceIsRoot(bd *ec2.InstanceBlockDeviceMapping, instance *ec2.Instance) bool {
	return bd.DeviceName != nil &&
		instance.RootDeviceName != nil &&
		*bd.DeviceName == *instance.RootDeviceName
}

func fetchRootDeviceName(ami string, conn *ec2.EC2) (*string, error) {
	if ami == "" {
		return nil, errors.New("Cannot fetch root device name for blank AMI ID.")
	}

	log.Printf("[DEBUG] Describing AMI %q to get root block device name", ami)
	res, err := conn.DescribeImages(&ec2.DescribeImagesInput{
		ImageIds: []*string{aws.String(ami)},
	})
	if err != nil {
		return nil, err
	}

	// For a bad image, we just return nil so we don't block a refresh
	if len(res.Images) == 0 {
		return nil, nil
	}

	image := res.Images[0]
	rootDeviceName := image.RootDeviceName

	// Instance store backed AMIs do not provide a root device name.
	if *image.RootDeviceType == ec2.DeviceTypeInstanceStore {
		return nil, nil
	}

	// Some AMIs have a RootDeviceName like "/dev/sda1" that does not appear as a
	// DeviceName in the BlockDeviceMapping list (which will instead have
	// something like "/dev/sda")
	//
	// While this seems like it breaks an invariant of AMIs, it ends up working
	// on the AWS side, and AMIs like this are common enough that we need to
	// special case it so Terraform does the right thing.
	//
	// Our heuristic is: if the RootDeviceName does not appear in the
	// BlockDeviceMapping, assume that the DeviceName of the first
	// BlockDeviceMapping entry serves as the root device.
	rootDeviceNameInMapping := false
	for _, bdm := range image.BlockDeviceMappings {
		if bdm.DeviceName == image.RootDeviceName {
			rootDeviceNameInMapping = true
		}
	}

	if !rootDeviceNameInMapping && len(image.BlockDeviceMappings) > 0 {
		rootDeviceName = image.BlockDeviceMappings[0].DeviceName
	}

	if rootDeviceName == nil {
		return nil, fmt.Errorf("[WARN] Error finding Root Device Name for AMI (%s)", ami)
	}

	return rootDeviceName, nil
}

func readBlockDeviceMappingsFromConfig(
	d *schema.ResourceData, conn *ec2.EC2) ([]*ec2.BlockDeviceMapping, error) {
	blockDevices := make([]*ec2.BlockDeviceMapping, 0)

	if v, ok := d.GetOk("ebs_block_device"); ok {
		vL := v.(*schema.Set).List()
		for _, v := range vL {
			bd := v.(map[string]interface{})
			ebs := &ec2.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(bd["delete_on_termination"].(bool)),
			}

			if v, ok := bd["snapshot_id"].(string); ok && v != "" {
				ebs.SnapshotId = aws.String(v)
			}

			if v, ok := bd["encrypted"].(bool); ok && v {
				ebs.Encrypted = aws.Bool(v)
			}

			if v, ok := bd["volume_size"].(int); ok && v != 0 {
				ebs.VolumeSize = aws.Int64(int64(v))
			}

			if v, ok := bd["volume_type"].(string); ok && v != "" {
				ebs.VolumeType = aws.String(v)
			}

			if v, ok := bd["iops"].(int); ok && v > 0 {
				ebs.Iops = aws.Int64(int64(v))
			}

			blockDevices = append(blockDevices, &ec2.BlockDeviceMapping{
				DeviceName: aws.String(bd["device_name"].(string)),
				Ebs:        ebs,
			})
		}
	}

	if v, ok := d.GetOk("ephemeral_block_device"); ok {
		vL := v.(*schema.Set).List()
		for _, v := range vL {
			bd := v.(map[string]interface{})
			bdm := &ec2.BlockDeviceMapping{
				DeviceName:  aws.String(bd["device_name"].(string)),
				VirtualName: aws.String(bd["virtual_name"].(string)),
			}
			if v, ok := bd["no_device"].(bool); ok && v {
				bdm.NoDevice = aws.String("")
				// When NoDevice is true, just ignore VirtualName since it's not needed
				bdm.VirtualName = nil
			}

			if bdm.NoDevice == nil && aws.StringValue(bdm.VirtualName) == "" {
				return nil, errors.New("virtual_name cannot be empty when no_device is false or undefined.")
			}

			blockDevices = append(blockDevices, bdm)
		}
	}

	if v, ok := d.GetOk("root_block_device"); ok {
		vL := v.([]interface{})
		if len(vL) > 1 {
			return nil, errors.New("Cannot specify more than one root_block_device.")
		}
		for _, v := range vL {
			bd := v.(map[string]interface{})
			ebs := &ec2.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(bd["delete_on_termination"].(bool)),
			}

			if v, ok := bd["volume_size"].(int); ok && v != 0 {
				ebs.VolumeSize = aws.Int64(int64(v))
			}

			if v, ok := bd["volume_type"].(string); ok && v != "" {
				ebs.VolumeType = aws.String(v)
			}

			if v, ok := bd["iops"].(int); ok && v > 0 && *ebs.VolumeType == "io1" {
				// Only set the iops attribute if the volume type is io1. Setting otherwise
				// can trigger a refresh/plan loop based on the computed value that is given
				// from AWS, and prevent us from specifying 0 as a valid iops.
				//   See https://github.com/hashicorp/terraform/pull/4146
				//   See https://github.com/hashicorp/terraform/issues/7765
				ebs.Iops = aws.Int64(int64(v))
			} else if v, ok := bd["iops"].(int); ok && v > 0 && *ebs.VolumeType != "io1" {
				// Message user about incompatibility
				log.Print("[WARN] IOPs is only valid for storate type io1 for EBS Volumes")
			}

			if dn, err := fetchRootDeviceName(d.Get("ami").(string), conn); err == nil {
				if dn == nil {
					return nil, fmt.Errorf(
						"Expected 1 AMI for ID: %s, got none",
						d.Get("ami").(string))
				}

				blockDevices = append(blockDevices, &ec2.BlockDeviceMapping{
					DeviceName: dn,
					Ebs:        ebs,
				})
			} else {
				return nil, err
			}
		}
	}

	return blockDevices, nil
}

// Determine whether we're referring to security groups with
// IDs or names. We use a heuristic to figure this out. By default,
// we use IDs if we're in a VPC. However, if we previously had an
// all-name list of security groups, we use names. Or, if we had any
// IDs, we use IDs.
func readSecurityGroups(d *schema.ResourceData, instance *ec2.Instance) error {
	useID := instance.SubnetId != nil && *instance.SubnetId != ""
	if v := d.Get("security_groups"); v != nil {
		match := useID
		sgs := v.(*schema.Set).List()
		if len(sgs) > 0 {
			match = false
			for _, v := range v.(*schema.Set).List() {
				if strings.HasPrefix(v.(string), "sg-") {
					match = true
					break
				}
			}
		}

		useID = match
	}

	// Build up the security groups
	sgs := make([]string, 0, len(instance.SecurityGroups))
	if useID {
		for _, sg := range instance.SecurityGroups {
			sgs = append(sgs, *sg.GroupId)
		}
		log.Printf("[DEBUG] Setting Security Group IDs: %#v", sgs)
		if err := d.Set("vpc_security_group_ids", sgs); err != nil {
			return err
		}
		if err := d.Set("security_groups", []string{}); err != nil {
			return err
		}
	} else {
		for _, sg := range instance.SecurityGroups {
			sgs = append(sgs, *sg.GroupName)
		}
		log.Printf("[DEBUG] Setting Security Group Names: %#v", sgs)
		if err := d.Set("security_groups", sgs); err != nil {
			return err
		}
		if err := d.Set("vpc_security_group_ids", []string{}); err != nil {
			return err
		}
	}
	return nil
}

type awsInstanceOpts struct {
	BlockDeviceMappings               []*ec2.BlockDeviceMapping
	DisableAPITermination             *bool
	EBSOptimized                      *bool
	Monitoring                        *ec2.RunInstancesMonitoringEnabled
	IAMInstanceProfile                *ec2.IamInstanceProfileSpecification
	ImageID                           *string
	InstanceInitiatedShutdownBehavior *string
	InstanceType                      *string
	KeyName                           *string
	NetworkInterfaces                 []*ec2.InstanceNetworkInterfaceSpecification
	Placement                         *ec2.Placement
	PrivateIPAddress                  *string
	SecurityGroupIDs                  []*string
	SecurityGroups                    []*string
	SpotPlacement                     *ec2.SpotPlacement
	SubnetID                          *string
	UserData64                        *string
}

func buildAwsInstanceOpts(
	d *schema.ResourceData, meta interface{}) (*awsInstanceOpts, error) {
	conn := meta.(*AWSClient).ec2conn

	opts := &awsInstanceOpts{
		DisableAPITermination: aws.Bool(d.Get("disable_api_termination").(bool)),
		EBSOptimized:          aws.Bool(d.Get("ebs_optimized").(bool)),
		ImageID:               aws.String(d.Get("ami").(string)),
		InstanceType:          aws.String(d.Get("instance_type").(string)),
	}

	if v := d.Get("instance_initiated_shutdown_behavior").(string); v != "" {
		opts.InstanceInitiatedShutdownBehavior = aws.String(v)
	}

	opts.Monitoring = &ec2.RunInstancesMonitoringEnabled{
		Enabled: aws.Bool(d.Get("monitoring").(bool)),
	}

	opts.IAMInstanceProfile = &ec2.IamInstanceProfileSpecification{
		Name: aws.String(d.Get("iam_instance_profile").(string)),
	}

	user_data := d.Get("user_data").(string)

	opts.UserData64 = aws.String(base64Encode([]byte(user_data)))

	// check for non-default Subnet, and cast it to a String
	subnet, hasSubnet := d.GetOk("subnet_id")
	subnetID := subnet.(string)

	// Placement is used for aws_instance; SpotPlacement is used for
	// aws_spot_instance_request. They represent the same data. :-|
	opts.Placement = &ec2.Placement{
		AvailabilityZone: aws.String(d.Get("availability_zone").(string)),
		GroupName:        aws.String(d.Get("placement_group").(string)),
	}

	opts.SpotPlacement = &ec2.SpotPlacement{
		AvailabilityZone: aws.String(d.Get("availability_zone").(string)),
		GroupName:        aws.String(d.Get("placement_group").(string)),
	}

	if v := d.Get("tenancy").(string); v != "" {
		opts.Placement.Tenancy = aws.String(v)
	}

	associatePublicIPAddress := d.Get("associate_public_ip_address").(bool)

	var groups []*string
	if v := d.Get("security_groups"); v != nil {
		// Security group names.
		// For a nondefault VPC, you must use security group IDs instead.
		// See http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_RunInstances.html
		sgs := v.(*schema.Set).List()
		if len(sgs) > 0 && hasSubnet {
			log.Print("[WARN] Deprecated. Attempting to use 'security_groups' within a VPC instance. Use 'vpc_security_group_ids' instead.")
		}
		for _, v := range sgs {
			str := v.(string)
			groups = append(groups, aws.String(str))
		}
	}

	if hasSubnet && associatePublicIPAddress {
		// If we have a non-default VPC / Subnet specified, we can flag
		// AssociatePublicIpAddress to get a Public IP assigned. By default these are not provided.
		// You cannot specify both SubnetId and the NetworkInterface.0.* parameters though, otherwise
		// you get: Network interfaces and an instance-level subnet ID may not be specified on the same request
		// You also need to attach Security Groups to the NetworkInterface instead of the instance,
		// to avoid: Network interfaces and an instance-level security groups may not be specified on
		// the same request
		ni := &ec2.InstanceNetworkInterfaceSpecification{
			AssociatePublicIpAddress: aws.Bool(associatePublicIPAddress),
			DeviceIndex:              aws.Int64(int64(0)),
			SubnetId:                 aws.String(subnetID),
			Groups:                   groups,
		}

		if v, ok := d.GetOk("private_ip"); ok {
			ni.PrivateIpAddress = aws.String(v.(string))
		}

		if v := d.Get("vpc_security_group_ids").(*schema.Set); v.Len() > 0 {
			for _, v := range v.List() {
				ni.Groups = append(ni.Groups, aws.String(v.(string)))
			}
		}

		opts.NetworkInterfaces = []*ec2.InstanceNetworkInterfaceSpecification{ni}
	} else {
		if subnetID != "" {
			opts.SubnetID = aws.String(subnetID)
		}

		if v, ok := d.GetOk("private_ip"); ok {
			opts.PrivateIPAddress = aws.String(v.(string))
		}
		if opts.SubnetID != nil &&
			*opts.SubnetID != "" {
			opts.SecurityGroupIDs = groups
		} else {
			opts.SecurityGroups = groups
		}

		if v := d.Get("vpc_security_group_ids").(*schema.Set); v.Len() > 0 {
			for _, v := range v.List() {
				opts.SecurityGroupIDs = append(opts.SecurityGroupIDs, aws.String(v.(string)))
			}
		}
	}

	if v, ok := d.GetOk("key_name"); ok {
		opts.KeyName = aws.String(v.(string))
	}

	blockDevices, err := readBlockDeviceMappingsFromConfig(d, conn)
	if err != nil {
		return nil, err
	}
	if len(blockDevices) > 0 {
		opts.BlockDeviceMappings = blockDevices
	}

	return opts, nil
}

func awsTerminateInstance(conn *ec2.EC2, id string) error {
	log.Printf("[INFO] Terminating instance: %s", id)
	req := &ec2.TerminateInstancesInput{
		InstanceIds: []*string{aws.String(id)},
	}
	if _, err := conn.TerminateInstances(req); err != nil {
		return fmt.Errorf("Error terminating instance: %s", err)
	}

	log.Printf("[DEBUG] Waiting for instance (%s) to become terminated", id)

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Target:     []string{"terminated"},
		Refresh:    InstanceStateRefreshFunc(conn, id),
		Timeout:    10 * time.Minute,
		Delay:      10 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err := stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf(
			"Error waiting for instance (%s) to terminate: %s", id, err)
	}

	return nil
}

func iamInstanceProfileArnToName(ip *ec2.IamInstanceProfile) string {
	if ip == nil || ip.Arn == nil {
		return ""
	}
	parts := strings.Split(*ip.Arn, "/")
	return parts[len(parts)-1]
}

func userDataHashSum(user_data string) string {
	// Check whether the user_data is not Base64 encoded.
	// Always calculate hash of base64 decoded value since we
	// check against double-encoding when setting it
	v, base64DecodeError := base64.StdEncoding.DecodeString(user_data)
	if base64DecodeError != nil {
		v = []byte(user_data)
	}

	hash := sha1.Sum(v)
	return hex.EncodeToString(hash[:])
}
