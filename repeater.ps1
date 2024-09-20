Function Log($msg) {
	$date = Get-Date -Format "yyyy/MM/dd HH:mm:ss"
	$host.ui.WriteErrorLine("[W] $date $msg")
}

Function ReadMessage($stream) {
	$buf = New-Object byte[] 4
	$offset = 0
	while ($offset -lt 4) {
		$n = $stream.Read($buf, $offset, 4 - $offset);
		if ($n -eq 0) {
			break
		}
		$offset += $n;
	}
	if ($offset -eq 4) {
		$len = (($buf[0] * 256 + $buf[1]) * 256 + $buf[2]) * 256 + $buf[3] + 4
		[Array]::Resize([ref]$buf, $len)
		while ($offset -lt $buf.Length) {
			$n = $stream.Read($buf, $offset, $buf.Length - $offset)
			if ($n -eq 0) {
				break
			}
			$offset += $n
		}
	}
	[Array]::Resize([ref]$buf, $offset)	
	return $buf
}

Function MainLoop {
	Try {
		$ignoreOpenSSHExtensions = $false
		Try {
			$sshAgentVersion = (Get-Command -CommandType Application ssh-agent.exe -ErrorAction Stop)[0].Version
			$ignoreOpenSSHExtensions = ($sshAgentVersion.Major -le 8 -and $sshAgentVersion.Minor -lt 9)
			Log "ssh-agent.exe version: $($sshAgentVersion.ToString()) (ignoreOpenSSHExtensions: $ignoreOpenSSHExtensions)"
		}
		Catch {
			$ignoreOpenSSHExtensions = $true
		}

		$ssh_client_in = [console]::OpenStandardInput()
		$ssh_client_out = [console]::OpenStandardOutput()

		$ver = $PSVersionTable["PSVersion"]
		$ssh_client_out.WriteByte(0xff)
		Log "ready: PSVersion $ver"

		$buf = ReadMessage $ssh_client_in
		$pipename = [System.Text.Encoding]::UTF8.GetString($buf[4..$buf.Length])
		Log "[W] named pipe: $pipename"

		while ($true) {
			Try {
				$null = $ssh_client_in.Read((New-Object byte[] 1), 0, 0)
				$buf = ReadMessage $ssh_client_in
				if ($ignoreOpenSSHExtensions -and $buf.Length -gt 4 -and $buf[4] -eq 0x1b) {
					$buf = [byte[]](0, 0, 0, 1, 6)
					$ssh_client_out.Write($buf, 0, $buf.Length)
					Log "[W] return dummy for OpenSSH ext."
					Continue
				}
				$ssh_agent = New-Object System.IO.Pipes.NamedPipeClientStream ".", $pipename, InOut
				$ssh_agent.Connect()
				Log "[W] named pipe: connected"
				$ssh_agent.Write($buf, 0, $buf.Length)
				Log "[L] -> [W] -> ssh-agent.exe ($($buf.Length) B)"
				$buf = ReadMessage $ssh_agent
				$ssh_client_out.Write($buf, 0, $buf.Length)
				Log "[L] <- [W] <- ssh-agent.exe ($($buf.Length) B)"
			}
			Finally {
				if ($null -ne $ssh_agent) {
					$ssh_agent.Dispose()
					Log "[W] named pipe: disconnected"
				}
			}
		}
	}
	Finally {
		$host.ui.WriteErrorLine("wsl2-ssh-agent.ps1: terminated")
	}
}

MainLoop
