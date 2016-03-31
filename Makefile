FREY_VERSION := 0.3.13

PHONY: frey
frey:
	@grep $(FREY_VERSION) node_modules/frey/package.json 2>&1 > /dev/null || npm install frey@$(FREY_VERSION)

PHONY: provision
provision: frey
	@source env.sh && node_modules/.bin/frey install

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

PHONY: console-localfrey
console-localfrey:
	@cd .infra && source env.sh && babel-node ${HOME}/code/frey/src/cli.js remote
