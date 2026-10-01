<?php

// phar2go's replacement for bStats: plugin statistics aren't sent from pocketmine-go.

namespace __NS__;

use pocketmine\plugin\Plugin;
use __NS__\charts\CustomChart;

class Metrics{
	public function __construct(Plugin $plugin, int $pluginId, bool $logFailedRequests = false){
	}

	public function addCustomChart(CustomChart $chart) : void{
	}

	public function shutdown() : void{
	}
}
