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
      "us-east-1" = "ami-9bce7af0"
    }
  }
  region {
    default = "us-east-1"
  }
}

infra output {
  public_address {
    value = "${aws_instance.tusd.0.public_dns}"
  }
  public_addresses {
    value = "${join("\n", aws_instance.tusd.*.public_dns)}"
  }
  endpoint {
    value = "http://${aws_route53_record.www.name}:80/"
  }
}

infra resource aws_key_pair "infra-tusd-main" {
  key_name   = "infra-tusd-main"
  public_key = "${file("{{{config.global.ssh.publickey_file}}}")}"
}

infra resource aws_instance tusd {
  ami             = "${lookup(var.amis, var.region)}"
  instance_type   = "c3.large"
  key_name        = "${aws_key_pair.infra-tusd-main.key_name}"

  security_groups = ["fw-tusd-main"]
  connection {
    key_file = "{{{config.global.ssh.privatekey_file}}}"
    user     = "{{{config.global.ssh.user}}}"
  }
  tags {
    "Name" = "${var.FREY_DOMAIN}"
  }
}

infra resource "aws_route53_record" www {
  name    = "${var.FREY_DOMAIN}"
  records = ["${aws_instance.tusd.public_dns}"]
  ttl     = "300"
  type    = "CNAME"
  zone_id = "${var.FREY_AWS_ZONE_ID}"
}

infra resource aws_security_group "fw-tusd-main" {
  description = "Infra tusd"
  name        = "fw-tusd-main"
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    from_port   = 8080
    protocol    = "tcp"
    to_port     = 8080
  }
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    from_port   = 80
    protocol    = "tcp"
    to_port     = 80
  }
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    from_port   = 443
    protocol    = "tcp"
    to_port     = 443
  }
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
    from_port   = 22
    protocol    = "tcp"
    to_port     = 22
  }
}

install {
  playbooks {
    hosts = "tusd"
    name  = "Install tusd"
    roles {
      role        = "{{{init.paths.roles_dir}}}/apt/1.4.0"
      apt_install = [
        "apg",
        "build-essential",
        "curl",
        "git-core",
        "htop",
        "iotop",
        "libpcre3",
        "logtail",
        "mlocate",
        "mtr",
        "psmisc",
        "telnet",
        "vim",
        "wget"
      ]
    }
    roles {
      role = "{{{init.paths.roles_dir}}}/unattended-upgrades/1.3.1"
    }
    roles {
      role = "{{{init.paths.roles_dir}}}/fqdn/1.2.1"
    }
    roles {
      role = "{{{init.paths.roles_dir}}}/timezone/1.0.0"
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
  }
}

setup {
  playbooks {
    hosts = "tusd"
    name  = "Setup tusd"
    roles {
      role                  = "{{{init.paths.roles_dir}}}/upstart/1.0.0"
      upstart_command       = "./tusd -port=8080 -dir=/mnt/tusd-data -max-size=1000000000 -behind-proxy"
      upstart_description   = "tusd server"
      upstart_name          = "{{{config.global.appname}}}"
      upstart_pidfile_path  = "{{{config.global.approot}}}/shared/{{{config.global.appname}}}.pid"
      upstart_respawn       = true
      upstart_respawn_limit = true
      upstart_runtime_root  = "{{{config.global.approot}}}/current/tusd_linux_amd64"
      upstart_user          = "www-data"
    }
    roles {
      role = "{{{init.paths.roles_dir}}}/rsyslog/3.1.1"
      rsyslog_rsyslog_d_files "49-tusd" {
        directives = ["& stop"]
        rules {
          rule    = ":programname, startswith, \"{{{config.global.appname}}}\""
          logpath = "{{{config.global.approot}}}/shared/logs/{{{config.global.appname}}}.log"
        }
      }
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

  playbooks {
    hosts = "tusd"
    name  = "Setup nginx"
    tasks {
      name           = "nginx | Add nginx PPA"
      apt_repository = "repo='ppa:ondrej/nginx'"
    }
    tasks {
      name = "nginx | Create public www directory"
      file = "path=/mnt/nginx-www state=directory owner=www-data group=ubuntu mode=ug+rwX,o= recurse=yes"
    }
  }
}

deploy {
  playbooks {
    hosts = "tusd"
    name  = "Deploy tusd"
    roles {
      role                   = "{{{init.paths.roles_dir}}}/deploy/2.0.0"
      ansistrano_deploy_from = "./files/tusd_linux_amd64.tar.gz"
      ansistrano_deploy_to   = "{{{config.global.approot}}}"
      ansistrano_deploy_via  = "copy_unarchive"
      ansistrano_group       = "ubuntu"
    }
    tasks {
      file = "path=/srv/tusd/shared/logs state=directory owner=syslog group=ubuntu mode=ug+rwX,o= recurse=yes"
      name = "tusd | Create and chown shared log dir"
    }
    tasks {
      name = "tusd | Set file attributes"
      file = "path={{{config.global.approot}}}/current/tusd_linux_amd64/tusd mode=0755 owner=www-data group=www-data"
    }
  }
  playbooks {
    hosts = "tusd"
    name  = "Deploy nginx"
    roles {
      role        = "{{{init.paths.roles_dir}}}/apt/1.4.0"
      apt_install = ["nginx-light"]
    }
    tasks {
      name = "nginx | Create nginx configuration"
      copy = "src=./files/nginx.conf dest=/etc/nginx/sites-enabled/default"
    }
    tasks {
      name    = "nginx | Create DH parameters"
      command = "openssl dhparam -out /etc/nginx/dhparams.pem 2048 creates=/etc/nginx/dhparams.pem"
    }
    tasks {
      name = "nginx | Start service"
      service = "name=nginx state=started"
    }
  }
}

restart {
  playbooks {
    hosts = "tusd"
    name  = "Restart tusd"
    tasks {
      action = "service name=tusd state=restarted"
      name   = "tusd | Restart"
    }
  }
  playbooks {
    hosts = "tusd"
    name  = "Restart nginx"
    tasks {
      name    = "nginx | Restart"
      service = "name=nginx state=restarted"
    }
  }
}
