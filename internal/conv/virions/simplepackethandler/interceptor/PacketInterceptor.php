<?php

namespace __NS__\interceptor;

use Closure;

final class PacketInterceptor implements IPacketInterceptor{
	public function interceptIncoming(Closure $handler) : IPacketInterceptor{
		return $this;
	}

	public function interceptOutgoing(Closure $handler) : IPacketInterceptor{
		return $this;
	}

	public function unregisterIncomingInterceptor(Closure $handler) : IPacketInterceptor{
		return $this;
	}

	public function unregisterOutgoingInterceptor(Closure $handler) : IPacketInterceptor{
		return $this;
	}
}
