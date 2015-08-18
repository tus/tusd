variable "TSD_AWS_ACCESS_KEY" {}
variable "TSD_AWS_SECRET_KEY" {}
variable "TSD_AWS_ZONE_ID" {}
variable "TSD_DOMAIN" {}
variable "TSD_SSH_USER" {}
variable "TSD_SSH_KEY_FILE" {}
variable "TSD_SSH_KEY_NAME" {}

variable "ip_kevin" {
  default = "62.163.187.106/32"
}
variable "ip_marius" {
  default = "84.146.5.70/32"
}
variable "ip_tim" {
  default = "24.134.75.132/32"
}
variable "ip_all" {
  default = "0.0.0.0/0"
}

provider "aws" {
  access_key = "${var.TSD_AWS_ACCESS_KEY}"
  secret_key = "${var.TSD_AWS_SECRET_KEY}"
  region     = "us-east-1"
}

variable "ami" {
  // http://cloud-images.ubuntu.com/locator/ec2/
  default = {
    us-east-1 = "ami-9bce7af0" // us-east-1	trusty	14.04 LTS	amd64	ebs-ssd	20150814 ami-9bce7af0
  }
}

variable "region" {
  default     = "us-east-1"
  description = "The region of AWS, for AMI lookups."
}

resource "aws_instance" "infra-tusd-server" {
  ami             = "${lookup(var.ami, var.region)}"
  instance_type   = "c3.large"
  key_name        = "${var.TSD_SSH_KEY_NAME}"
  security_groups = [
    "fw-infra-tusd-main"
  ]

  connection {
    user     = "ubuntu"
    key_file = "${var.TSD_SSH_KEY_FILE}"
  }
}

resource "aws_route53_record" "www" {
  zone_id  = "${var.TSD_AWS_ZONE_ID}"
  name     = "${var.TSD_DOMAIN}"
  type     = "CNAME"
  ttl      = "300"
  records  = [ "${aws_instance.infra-tusd-server.public_dns}" ]
}

resource "aws_security_group" "fw-infra-tusd-main" {
  name        = "fw-infra-tusd-main"
  description = "Infra tusd"

  // SSH
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [
      "${var.ip_kevin}",
      "${var.ip_marius}",
      "${var.ip_tim}"
    ]
  }

  // Web
  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = [
      "${var.ip_all}"
    ]
  }
}

output "public_address" {
  value = "${aws_instance.infra-tusd-server.0.public_dns}"
}

output "public_addresses" {
  value = "${join(\"\n\", aws_instance.infra-tusd-server.*.public_dns)}"
}
