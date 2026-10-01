<?php

namespace __NS__\interceptor;

use Closure;

interface IPacketInterceptor{
	public function interceptIncoming(Closure $handler) : IPacketInterceptor;

	public function interceptOutgoing(Closure $handler) : IPacketInterceptor;

	public function unregisterIncomingInterceptor(Closure $handler) : IPacketInterceptor;

	public function unregisterOutgoingInterceptor(Closure $handler) : IPacketInterceptor;
}
