<?php

// phar2go's replacement for SimplePacketHandler: pocketmine-go's packets are gophertunnel's,
// so packet handlers can't be called with PocketMine-MP packet objects. Handlers are accepted
// and never called.

namespace __NS__;

use pocketmine\plugin\Plugin;
use __NS__\interceptor\IPacketInterceptor;
use __NS__\interceptor\PacketInterceptor;
use __NS__\monitor\IPacketMonitor;
use __NS__\monitor\PacketMonitor;

final class SimplePacketHandler{
	public static function createInterceptor(Plugin $registerer, int $priority = 0, bool $handle_cancelled = false) : IPacketInterceptor{
		$registerer->getLogger()->warning("Packet interceptors aren't supported on pocketmine-go; packet handlers won't be called");
		return new PacketInterceptor();
	}

	public static function createMonitor(Plugin $registerer, bool $handle_cancelled = false) : IPacketMonitor{
		$registerer->getLogger()->warning("Packet monitors aren't supported on pocketmine-go; packet handlers won't be called");
		return new PacketMonitor();
	}
}
