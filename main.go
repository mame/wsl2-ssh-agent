package main

func main() {
	c := newConfig()

	ctx := c.start()

	s := newServer(c.socketPath, c.powershellPath)

	s.run(ctx)
}
