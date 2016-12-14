global {
  appname = "tusd"
  approot = "/srv/tusd"
  ssh {
    key_dir = "./ssh"
  }
  ansiblecfg {
    privilege_escalation {
      become = true
    }
    defaults {
      host_key_checking = false
      ansible_managed = "Ansible managed"
    }
    ssh_connection {
      pipelining = true
    }
  }
}

infra provider aws {
  access_key = "${var.FREY_AWS_ACCESS_KEY}"
  region     = "us-east-1"
  secret_key = "${var.FREY_AWS_SECRET_KEY}"
}

infra variable {
  amis {
    type = "map"
    default {
      "us-east-1" = "ami-8fe79998"
    }
  }
  region {
    default = "us-east-1"
  }
}

infra output {
  public_address { value = "${aws_instance.tusd.0.public_dns}" }
  public_addresses { value = "${join("\n", aws_instance.tusd.*.public_dns)}" }
  endpoint { value = "http://${aws_route53_record.www.name}:80/" }
}

infra resource aws_key_pair "infra-tusd" {
  key_name   = "infra-tusd"
  public_key = "${file("{{{config.global.ssh.publickey_file}}}")}"
}

infra resource aws_instance "tusd" {
  ami                    = "${lookup(var.amis, var.region)}"
  instance_type          = "t2.micro"
  key_name               = "${aws_key_pair.infra-tusd.key_name}"
  // vpc_security_group_ids = ["aws_security_group.fw-tusd.id"]
  subnet_id              = "subnet-1adf3953"

  connection {
    key_file = "{{{config.global.ssh.privatekey_file}}}"
    user     = "{{{config.global.ssh.user}}}"
  }

  tags {
    Name = "${var.FREY_DOMAIN}"
  }
}

infra resource aws_route53_record "www" {
  name    = "${var.FREY_DOMAIN}"
  records = ["${aws_instance.tusd.public_dns}"]
  ttl     = "300"
  type    = "CNAME"
  zone_id = "${var.FREY_AWS_ZONE_ID}"
}

infra resource aws_security_group "fw-tusd" {
  description = "Infra tusd"
  name        = "fw-tusd"
  vpc_id      = "vpc-cea030a9"

  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    protocol    = "tcp"
    from_port   = 8080
    to_port     = 8080
  }

  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    protocol    = "tcp"
    from_port   = 80
    to_port     = 80
  }

  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    protocol    = "tcp"
    from_port   = 22
    to_port     = 22
  }

  // This is for outbound internet access
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [ "0.0.0.0/0" ]
  }
}

install {
  playbooks {
    hosts = "tusd"
    name  = "Install tusd"

    roles {
      role         = "{{{init.paths.roles_dir}}}/apt/v1.0.0"
      apt_packages = ["apg", "build-essential", "curl", "git-core", "htop", "iotop", "libpcre3", "logtail", "mlocate", "mtr", "psmisc", "telnet", "vim", "wget"]
    }

    roles {
      role = "{{{init.paths.roles_dir}}}/unattended-upgrades/1.2.0"
    }

    tasks {
      lineinfile = "dest=/home/{{{config.global.ssh.user}}}/.bashrc line=\"alias wtf='sudo tail -f /var/log/*{log,err} /var/log/{dmesg,messages,*{,/*}{log,err}}'\" owner={{{config.global.ssh.user}}} group={{{config.global.ssh.user}}} mode=0644 backup=yes"
      name       = "Common | Add convenience shortcut wtf"
    }

    tasks {
      lineinfile = "dest=/home/{{{config.global.ssh.user}}}/.bashrc line=\"cd {{{config.global.approot}}}/current || true\" owner={{{config.global.ssh.user}}} group={{{config.global.ssh.user}}} mode=0644 backup=yes"
      name       = "Common | Install login"
    }

    tasks {
      name = "Common | Set motd"
      copy = "content='Welcome to {{lookup('env', 'FREY_DOMAIN')}}' dest=/etc/motd owner=root group=root mode=0644 backup=yes"
    }

    tasks {
      name   = "Common | Set timezone variables"
      copy   = "content='Etc/UTC' dest=/etc/timezone owner=root group=root mode=0644 backup=yes"
      notify = ["Common | Update timezone"]
    }

    tasks {
      name       = "Common | Disable UseDNS for SSHD"
      lineinfile = "dest=/etc/ssh/sshd_config regexp=\"^UseDNS\" line=\"UseDNS no\" state=present"
      notify     = ["Common | Restart sshd"]
    }

    handlers {
      name    = "Common | Update timezone"
      command = "dpkg-reconfigure --frontend noninteractive tzdata"
    }

    handlers {
      name    = "Common | Restart sshd"
      service = "name=ssh state=restarted"
    }
  }
}

setup {
  playbooks {
    hosts = "tusd"
    name  = "Setup tusd"

    roles {
      role                  = "{{{init.paths.roles_dir}}}/upstart/1.0.0"
      upstart_command       = "./tusd -port=8080 -dir=/mnt/tusd-data -store-size=10737418240"
      upstart_description   = "tusd server"
      upstart_name          = "{{{config.global.appname}}}"
      upstart_pidfile_path  = "{{{config.global.approot}}}/shared/{{{config.global.appname}}}.pid"
      upstart_respawn       = true
      upstart_respawn_limit = true
      upstart_runtime_root  = "{{{config.global.approot}}}/current/tusd_linux_amd64"
      upstart_user          = "www-data"
    }

    roles {
      role = "{{{init.paths.roles_dir}}}/rsyslog/3.1.0"
      rsyslog_rsyslog_d_files "49-tusd" {
        directives = ["& stop"]
        rules {
          rule    = ":programname, startswith, \"{{{config.global.appname}}}\""
          logpath = "{{{config.global.approot}}}/shared/logs/{{{config.global.appname}}}.log"
        }
      }
    }

    roles {
      role = "{{{init.paths.roles_dir}}}/fqdn/1.0.0"
      fqdn = "{{lookup('env', 'FREY_DOMAIN')}}"
    }

    tasks {
      file = "path=/mnt/tusd-data state=directory owner=www-data group=ubuntu mode=ug+rwX,o= recurse=yes"
      name = "tusd | Create tusd data dir"
    }

    tasks {
      name = "tusd | Create purger crontab (clean up >24h (1400minutes) files)"
      cron {
        name         = "purger"
        special_time = "hourly"
        job          = "find /mnt/tusd-data -type f -mmin +1440 -print0 | xargs -n 200 -r -0 rm || true"
      }
    }
  }
}

deploy {
  playbooks {
    hosts = "tusd"
    name  = "Deploy tusd"

    roles {
      role                  = "{{{init.paths.roles_dir}}}/deploy/1.4.0"
      ansistrano_get_url    = "https://github.com/tus/tusd/releases/download/0.5.2/tusd_linux_amd64.tar.gz"
      ansistrano_deploy_to  = "{{{config.global.approot}}}"
      ansistrano_deploy_via = "download_unarchive"
      ansistrano_group      = "ubuntu"
    }

    tasks {
      name = "tusd | Set file attributes"
      file = "path={{{config.global.approot}}}/current/tusd_linux_amd64/tusd mode=0755 owner=www-data group=www-data"
    }
  }
}

restart {
  playbooks {
    hosts = "tusd"
    name  = "Restart tusd"

    tasks {
      shell = "iptables -t nat -A PREROUTING -i eth0 -p tcp --dport 80 -j REDIRECT --to-port 8080"
      name  = "tusd | Redirect HTTP traffic to tusd"
    }

    tasks {
      action = "service name=tusd state=restarted"
      name   = "tusd | Restart"
    }
  }
}
