terraform {
  required_providers {
    tencentcloud = {
      source = "tencentcloudstack/tencentcloud"
    }
  }
}

provider "tencentcloud" {
  secret_id  = "XX"
  secret_key = "XX"
  region     = "ap-hongkong"
}

data "tencentcloud_images" "default" {
  image_type = ["PUBLIC_IMAGE"]
  os_name    = "ubuntu"
}

data "tencentcloud_instance_types" "default" {
  filter {
    name   = "instance-family"
    values = ["S5"]
  }

  cpu_core_count = 1
  memory_size    = 2
}

data "tencentcloud_availability_zones" "default" {
}

# resource "tencentcloud_instance" "old" {}

resource "tencentcloud_vpc" "default" {
  cidr_block = "10.0.0.0/16"
  name       = "dev"
}

resource "tencentcloud_subnet" "default" {
  vpc_id            = tencentcloud_vpc.default.id
  availability_zone = data.tencentcloud_availability_zones.default.zones.1.name
  name              = "dev"
  cidr_block        = "10.0.1.0/24"
}

resource "tencentcloud_instance" "default" {
  instance_name                           = "dev"
  availability_zone                       = data.tencentcloud_availability_zones.default.zones.1.name
  image_id                                = "img-22trbn9x"
  instance_type                           = "S5.SMALL2"
  allocate_public_ip                      = true
  instance_charge_type_prepaid_renew_flag = "NOTIFY_AND_MANUAL_RENEW"
  internet_charge_type                    = "TRAFFIC_POSTPAID_BY_HOUR"
  internet_max_bandwidth_out              = 5
  vpc_id                                  = tencentcloud_vpc.default.id
  subnet_id                               = tencentcloud_subnet.default.id
  system_disk_type                        = "CLOUD_PREMIUM"
  system_disk_size                        = 50
}
