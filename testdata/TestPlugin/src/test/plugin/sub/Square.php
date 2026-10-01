<?php

declare(strict_types=1);

namespace test\plugin\sub;

class Square extends Shape{
	public function __construct(private float $side){}

	public function area() : float{
		return $this->side ** 2;
	}
}
