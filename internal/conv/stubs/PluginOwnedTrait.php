<?php

namespace pocketmine\plugin;

trait PluginOwnedTrait{
	private ?Plugin $owningPlugin = null;

	public function getOwningPlugin() : Plugin{
		return $this->owningPlugin;
	}
}
