# Deploying Infra

## Intro

This repository can create a tusd server from scratch, following this flow:

```yaml
 - prepare: Install prerequisites
 - init   : Refreshes current infra state and saves to terraform.tfstate
 - plan   : Shows infra changes and saves in an executable plan
 - backup : Backs up server state
 - launch : Launches virtual machines at a provider (if needed) using Terraform's ./infra.tf
 - install: Runs Ansible to install software packages & configuration templates
 - upload : Upload the application
 - setup  : Runs the ./playbook/setup.sh remotely, installing app dependencies and starting it
 - show   : Displays active platform
```

## Important files

 - [envs/production/infra.tf](envs/production/infra.tf) responsible for creating server/ram/cpu/dns
 - [playbook/playbook.yml](playbook/playbook.yml) responsible for installing APT packages
 - [control.sh](control.sh) executes each step of the flow in a logical order. Relies on Terraform and Ansible.
 - [Makefile](Makefile) provides convenience shortcuts such as `make deploy`. [Bash autocomplete](http://blog.jeffterrace.com/2012/09/bash-completion-for-mac-os-x.html) makes this sweet.
 - [env.example.sh](env.example.sh) should be copied to `env.sh` and contains the secret keys to the infra provider (amazon, google, digitalocean, etc). These keys are necessary to change infra, but not packers & config, as the SSH keys are included in this repo
 
 
Not included with this repo:

 - `envs/production/infra-tusd.pem`
 - `env.sh`
 
As these contain the keys to create new infra and ssh into the created servers.


## Demo

In this 2 minute demo:

 - first a server is provisioned 
 - the machine-type is changed from `c3.large` (2 cores) to `c3.xlarge` (4 cores)
 - `make deploy-infra`
 - it detects a change, replaces the server, and provisions it

![terrible](https://cloud.githubusercontent.com/assets/26752/9314635/64b6be5c-452a-11e5-8d00-74e0b023077e.gif)

as you see this is a very powerful way to set up many more servers, or deal with calamities. Since everything is in Git, changes can be reviewed, reverted, etc. `make deploy-infra`, done.

## Prerequisites

### Terraform

> Terraform can set up machines & other resources at nearly all cloud providers

Installed automatically by `control.sh prepare` if missing.

### terraform-inventory

> Passes the hosts that Terraform created to Ansible.
> Parses state file, converts that to Ansible inventory.

**On OSX**

brew install https://raw.github.com/adammck/terraform-inventory/master/homebrew/terraform-inventory.rb

**On Linux**

Either compile the Go build, or use https://github.com/Homebrew/linuxbrew and `brew install` as well.

### Ansible

> A pragmatic, standardized way of provisioning servers with software & configuration.

**On OSX**

```bash
sudo -HE easy_install pip
sudo -HE pip install --upgrade pip
sudo -HE CFLAGS=-Qunused-arguments CPPFLAGS=-Qunused-arguments pip install --upgrade ansible
```

**On Linux**

```bash
sudo -HE easy_install pip
sudo -HE pip install --upgrade pip
sudo -HE pip install --upgrade ansible
```
