<?php

declare(strict_types=1);

namespace test\plugin;

use pocketmine\event\Listener;
use pocketmine\event\player\PlayerChatEvent;
use pocketmine\event\player\PlayerJoinEvent;

final class EventListener implements Listener{
	public function __construct(private Main $plugin){}

	/**
	 * @priority HIGH
	 */
	public function onJoin(PlayerJoinEvent $event) : void{
		$player = $event->getPlayer();
		$count = $this->plugin->addJoin($player);
		$player->sendMessage(Main::PREFIX . "Welcome #" . $count);
	}

	/**
	 * @handleCancelled
	 */
	public function onChat(PlayerChatEvent $event) : void{
		if(str_contains($event->getMessage(), "badword")){
			$event->cancel();
		}
	}
}
