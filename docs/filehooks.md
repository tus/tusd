# Hooks

When integrating tusd into an application, it is important to establish a communication channel between the two components. The tusd binary accomplishes this by providing a system when triggers actions when certain events happen, such as an upload being created or finished. This simple-but-powerful system enables uses ranging from logging over validate and authorization to processing the uploaded files. 

If you have previously worked with the hook system provided by [Git](https://git-scm.com/book/it/v2/Customizing-Git-Git-Hooks), you will see a lot of parallels. If this does not apply to you, don't worry, it is not complicated. Before getting stated, it is good to have a high level overview of what a hook is:

When a specific action happens during an upload (pre-create, post-receive, post-finish, or post-terminate), the hook system enables tusd to fire off a specific event. Tusd provides two ways of doing this:

1. Launch an arbitrary file, mirroring the Git hook system. 
2. Fire off an HTTP POST request to a custom endpoint. 

