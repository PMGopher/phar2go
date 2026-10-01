<?php

namespace pocketmine\utils;

trait LegacyEnumShimTrait{

	public static function __callStatic(string $name, array $arguments) : self{
		if(count($arguments) > 0){
			throw new \ArgumentCountError("Expected exactly 0 arguments, " . count($arguments) . " passed");
		}
		return self::getAll()[mb_strtoupper($name)];
	}

	public static function getAll() : array{
		static $result = null;
		if($result === null){
			$result = [];
			foreach(self::cases() as $case){
				$result[mb_strtoupper($case->name)] = $case;
			}
		}
		return $result;
	}

	public function name() : string{
		return $this->name;
	}

	public function id() : int{
		return spl_object_id($this);
	}

	public function equals(self $other) : bool{
		return $this === $other;
	}
}
