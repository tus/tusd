
.PHONY: deploy-infra
deploy-infra:
	# Sets up all local & remote dependencies. Useful for first-time uses
	# and to apply infra / software changes.
	@git checkout master
	@test -z "$$(git status --porcelain)" || (echo "Please first commit/clean your Git working directory" && false)
	@git pull
	source env.sh && ./control.sh prepare

.PHONY: deploy-infra-unsafe
deploy-infra-unsafe:
	# Sets up all local & remote dependencies. Useful for first-time uses
	# and to apply infra / software changes.
	# Does not check git index
	@git checkout master
	@git pull
	source env.sh && ./control.sh prepare

.PHONY: deploy
deploy:
	# For regular use. Just uploads the code and restarts the services
	@git checkout master
	@test -z "$$(git status --porcelain)" || (echo "Please first commit/clean your Git working directory" && false)
	@git pull
	source env.sh && ./control.sh install

.PHONY: deploy-unsafe
deploy-unsafe:
	# Does not check git index
	@git checkout master
	@git pull
	source env.sh && ./control.sh install

.PHONY: backup
backup:
	source env.sh && ./control.sh backup

.PHONY: restore
restore:
	source env.sh && ./control.sh restore

.PHONY: facts
facts:
	source env.sh && ./control.sh facts

.PHONY: console
console:
	source env.sh && ./control.sh remote
