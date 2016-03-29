PHONY: frey
frey:
	@npm install --global frey@0.3.12

PHONY: deploy
deploy:
	@cd .infra && source env.sh && frey setup

PHONY: launch
launch:
	@cd .infra && source env.infra.sh && frey infra

PHONY: console
console:
	@cd .infra && source env.sh && frey remote

PHONY: deploy-localfrey
deploy-localfrey:
	@cd .infra && source env.sh && babel-node ${HOME}/code/frey/src/cli.js setup
