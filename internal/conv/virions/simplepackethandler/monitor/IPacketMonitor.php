<?php

namespace __NS__\monitor;

use Closure;

interface IPacketMonitor{
	public function monitorIncoming(Closure $handler) : IPacketMonitor;

	public function monitorOutgoing(Closure $handler) : IPacketMonitor;

	public function unregisterIncomingMonitor(Closure $handler) : IPacketMonitor;

	public function unregisterOutgoingMonitor(Closure $handler) : IPacketMonitor;
}
