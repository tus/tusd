# Run plugin hook exemple

    make

    tusd -hooks-http=./hook_handler -hooks-enabled-events=pre-create,pre-finish,pre-access,post-create,post-receive,post-terminate,post-finish

Adapt enabled-events hooks list for your needs.
