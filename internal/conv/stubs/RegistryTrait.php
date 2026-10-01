<?php

namespace pocketmine\utils;

trait RegistryTrait{
	private static $members = null;

	private static function verifyName(string $name) : void{
		if(preg_match('/^(?!\d)[A-Za-z\d_]+$/u', $name) === 0){
			throw new \InvalidArgumentException("Invalid member name \"$name\", should only contain letters, numbers and underscores, and must not start with a number");
		}
	}

	private static function _registryRegister(string $name, object $member) : void{
		if(self::$members === null){
			throw new \LogicException("Cannot register members outside of " . self::class . "::setup()");
		}
		$upperName = mb_strtoupper($name);
		if(isset(self::$members[$upperName])){
			throw new \InvalidArgumentException("\"$upperName\" is already reserved");
		}
		self::$members[$upperName] = $member;
	}

	protected static function checkInit() : void{
		if(self::$members === null){
			self::$members = [];
			self::setup();
		}
	}

	private static function _registryFromString(string $name) : object{
		self::checkInit();
		$upperName = mb_strtoupper($name);
		if(!isset(self::$members[$upperName])){
			throw new \InvalidArgumentException("No such registry member: " . self::class . "::" . $upperName);
		}
		return self::preprocessMember(self::$members[$upperName]);
	}

	protected static function preprocessMember(object $member) : object{
		return $member;
	}

	public static function __callStatic($name, $arguments){
		try{
			return self::_registryFromString($name);
		}catch(\InvalidArgumentException $e){
			throw new \Error($e->getMessage(), 0, $e);
		}
	}

	private static function _registryGetAll() : array{
		self::checkInit();
		$result = [];
		foreach(self::$members as $name => $member){
			$result[$name] = self::preprocessMember($member);
		}
		return $result;
	}
}

trait CloningRegistryTrait{
	use RegistryTrait;

	protected static function preprocessMember(object $member) : object{
		return clone $member;
	}
}

trait EnumTrait{
	use RegistryTrait;

	protected static function register(self $member) : void{
		self::_registryRegister($member->name(), $member);
	}

	protected static function registerAll(self ...$members) : void{
		foreach($members as $member){
			self::register($member);
		}
	}

	public static function getAll() : array{
		return self::_registryGetAll();
	}

	private static $nextId = null;

	private $enumName;
	private $runtimeId;

	private function __construct(string $enumName){
		self::verifyName($enumName);
		$this->enumName = $enumName;
		if(self::$nextId === null){
			self::$nextId = 1;
		}
		$this->runtimeId = self::$nextId++;
	}

	public function name() : string{
		return $this->enumName;
	}

	public function id() : int{
		return $this->runtimeId;
	}

	public function equals(self $other) : bool{
		return $this->enumName === $other->enumName;
	}
}
