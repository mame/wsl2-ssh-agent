package main

func main() {
	c := newConfig()

	ctx, ignoreOpenSSHExtensions := c.start()

	s := newServer(c.socketPath, ignoreOpenSSHExtensions)

	s.run(ctx)
}
