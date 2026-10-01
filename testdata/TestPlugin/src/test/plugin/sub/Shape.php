<?php

declare(strict_types=1);

namespace test\plugin\sub;

abstract class Shape implements \JsonSerializable{
	abstract public function area() : float;

	public function name() : string{
		return static::class . "(" . $this->area() . ")";
	}

	public function jsonSerialize() : array{
		return ["name" => $this->name(), "area" => $this->area()];
	}
}
