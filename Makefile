FREY_VERSION := 0.3.23

.PHONY: frey
frey:
	@cd .infra && mkdir -p node_modules
	@cd .infra && (grep $(FREY_VERSION) node_modules/frey/package.json 2>&1 > /dev/null || npm install frey@$(FREY_VERSION))

.PHONY: provision
provision: frey
	@cd .infra && source env.sh && node_modules/.bin/frey install

.PHONY: deploy
deploy: frey
	@cd .infra && source env.sh && node_modules/.bin/frey setup

.PHONY: launch
launch: frey
	@cd .infra && source env.infra.sh && node_modules/.bin/frey infra

.PHONY: console
console: frey
	@cd .infra && source env.sh && node_modules/.bin/frey remote

.PHONY: deploy-localfrey
deploy-localfrey:
	@cd .infra && source env.sh && babel-node ${HOME}/code/frey/src/cli.js setup

.PHONY: console-localfrey
console-localfrey:
	@cd .infra && source env.sh && babel-node ${HOME}/code/frey/src/cli.js remote
