# wsl2-ssh-agent

This tool allows from WSL2 to use the ssh-agent service on Windows host.

## How to use

### 1. Install wsl2-ssh-agent on WSL2

```
go install https://github.com/mame/wsl2-ssh-agent@latest
```

And you will see `$HOME/go/bin/wsl2-ssh-agent`.

### 2. Modify .bashrc

Just add the following line to .bashrc.

```
eval $($HOME/go/bin/wsl2-ssh-agent)
```

Close and reopen the terminal and execute `ssh your-machine`.
The command should communicate with ssh-agent.exe service.

## Troubleshooting

*TBD*