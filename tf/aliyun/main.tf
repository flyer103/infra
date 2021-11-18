provider "alicloud" {
  access_key = "XX"
  secret_key = "XX"
  region     = "cn-wulanchabu"
}

variable "vpc_name" {
  default = "develop"
}

variable "vswitch_name" {
  default = "develop"
}

variable "ask_name" {
  default = "develop"
}

data "alicloud_zones" "default" {
  available_resource_creation = "VSwitch"
}

data "alicloud_cs_serverless_kubernetes_clusters" "k8s_clusters" {
  name_regex  = "dev"
  output_file = "dev-json"
}

output "output" {
  value = data.alicloud_cs_serverless_kubernetes_clusters.k8s_clusters.clusters
}

resource "alicloud_vpc" "default" {
  vpc_name   = var.vpc_name
  cidr_block = "172.16.0.0/12"
}

resource "alicloud_vswitch" "default" {
  vswitch_name = var.vswitch_name
  vpc_id       = alicloud_vpc.default.id
  cidr_block   = "172.16.2.0/24"
  zone_id      = data.alicloud_zones.default.zones[0].id
}

resource "alicloud_cs_serverless_kubernetes" "serverless" {
  name                           = var.ask_name
  vpc_id                         = alicloud_vpc.default.id
  vswitch_ids                    = [alicloud_vswitch.default.id]
  version                        = "v1.20.11-aliyun.1"
  new_nat_gateway                = true
  endpoint_public_access_enabled = true
  deletion_protection            = true
  kube_config                    = "~/.kube/config.dev.ask"

  load_balancer_spec      = "slb.s2.small"
  time_zone               = "Asia/Shanghai"
  service_cidr            = "172.21.0.0/20"
  service_discovery_types = ["CoreDNS"]

  addons {
    name = "metrics-server"
  }

  addons {
    name = "alb-ingress-controller"
  }
}
