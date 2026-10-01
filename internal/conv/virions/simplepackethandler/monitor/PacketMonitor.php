<?php

namespace __NS__\monitor;

use Closure;

final class PacketMonitor implements IPacketMonitor{
	public function monitorIncoming(Closure $handler) : IPacketMonitor{
		return $this;
	}

	public function monitorOutgoing(Closure $handler) : IPacketMonitor{
		return $this;
	}

	public function unregisterIncomingMonitor(Closure $handler) : IPacketMonitor{
		return $this;
	}

	public function unregisterOutgoingMonitor(Closure $handler) : IPacketMonitor{
		return $this;
	}
}
