<?php

declare(strict_types=1);

namespace test\plugin;

use pocketmine\command\Command;
use pocketmine\command\CommandSender;
use pocketmine\player\Player;
use pocketmine\plugin\PluginBase;
use pocketmine\scheduler\ClosureTask;
use pocketmine\utils\SingletonTrait;
use pocketmine\utils\TextFormat;
use test\plugin\sub\Counter;
use test\plugin\sub\Shape;
use test\plugin\sub\Square;

class Main extends PluginBase{
	use SingletonTrait;

	public const PREFIX = TextFormat::GREEN . "[Test] ";

	/** @var array<string, int> */
	private array $joins = [];
	private ?Counter $counter = null;

	protected function onLoad() : void{
		self::setInstance($this);
	}

	protected function onEnable() : void{
		$this->saveDefaultConfig();
		$this->counter = new Counter(3);
		$this->getServer()->getPluginManager()->registerEvents(new EventListener($this), $this);
		$interval = (int) $this->getConfig()->get("interval", 5);
		$this->getScheduler()->scheduleRepeatingTask(new ClosureTask(function() : void{
			$this->counter->increment();
		}), $interval * 20);
		$worlds = $this->getConfig()->get("worlds", []);
		if(!is_array($worlds) || count($worlds) !== 2){
			throw new \RuntimeException("worlds must be a list of two");
		}
		$shapes = [new Square(2.0), new Square(3.0)];
		$total = 0.0;
		foreach($shapes as $i => $shape){
			$total += $shape->area();
		}
		$this->getLogger()->info(self::PREFIX . sprintf("%.1f", $total) . " " . implode(",", array_map(fn(Shape $s) => $s->name(), $shapes)));
		try{
			$this->risky(-1);
		}catch(\InvalidArgumentException $e){
			$this->getLogger()->debug("caught: " . $e->getMessage());
		}finally{
			$this->joins["finally"] = 1;
		}
		$label = match(true){
			$total > 10 => "big",
			default => "small",
		};
		$this->getLogger()->info("Shapes are $label, counter {$this->counter->get()}");
		$gen = $this->counter->steps(3);
		$seen = [];
		while($gen->valid()){
			$seen[] = $gen->current();
			$gen->send(1);
		}
		$lazy = (function() : \Generator{
			yield "a" => 1;
			yield from ["b" => 2];
		})();
		foreach($lazy as $k => $v){
			$seen[] = "$k$v";
		}
		$this->getLogger()->info("Generator: " . implode(",", $seen) . " " . $gen->getReturn());
	}

	private function risky(int $n) : int{
		if($n < 0){
			throw new \InvalidArgumentException("negative: $n");
		}
		return $n * 2;
	}

	public function addJoin(Player $player) : int{
		$name = strtolower($player->getName());
		$this->joins[$name] = ($this->joins[$name] ?? 0) + 1;
		return $this->joins[$name];
	}

	public function onCommand(CommandSender $sender, Command $command, string $label, array $args) : bool{
		if(count($args) > 0 && $args[0] === "count"){
			$sender->sendMessage(self::PREFIX . "Counter: " . $this->counter->get());
			return true;
		}
		$name = $sender instanceof Player ? $sender->getName() : "console";
		$sender->sendMessage(str_replace("{player}", $name, (string) $this->getConfig()->get("greeting")));
		return true;
	}
}
