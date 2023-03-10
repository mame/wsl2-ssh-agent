Function Log($msg) {
	$date = Get-Date -Format "yyyy/MM/dd HH:mm:ss"
	$host.ui.WriteErrorLine("[W] $date $msg")
}

Function RelayMessage($from, $to, $buf, $arrow) {
	$offset = 0
	while ($offset -lt 4) {
		$n = $from.Read($buf, $offset, 4 - $offset);
		if ($n -eq 0) { exit }
		$offset += $n;
	}
	$len = (($buf[0] * 256 + $buf[1]) * 256 + $buf[2]) * 256 + $buf[3] + 4
	Log "[L] $arrow [W] $arrow ssh-agent.exe ($len B)"
	$len
	while ($offset -lt $len) {
		$n = $from.Read($buf, $offset, [Math]::Min($len, $buf.Length) - $offset)
		if ($n -eq 0) { exit }
		$offset += $n
		$to.Write($buf, 0, $offset)
		$len -= $offset
		$offset = 0
	}
}

Function MainLoop {
	Try {
		$ssh_agent = New-Object System.IO.Pipes.NamedPipeClientStream ".", "openssh-ssh-agent", InOut
		$ssh_agent.Connect()
		
		$buf = New-Object byte[] 8192
		$ssh_client_in = [console]::OpenStandardInput()
		$ssh_client_out = [console]::OpenStandardOutput()
	
		$ver = $PSVersionTable["PSVersion"]
		Log "ready: PSVersion $ver"

		$buf[0] = 0xff
		$ssh_client_out.Write($buf, 0, 1)
	
		while($true) {	
			$len = RelayMessage $ssh_client_in $ssh_agent $buf "->"
			$len = RelayMessage $ssh_agent $ssh_client_out $buf "<-"
		}	
	}
	Finally {
		$host.ui.WriteErrorLine("wsl2-ssh-agent.ps1: terminated")
	}
}

MainLoop
