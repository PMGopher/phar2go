<?php

declare(strict_types=1);

namespace test\plugin\sub;

final class Counter{
	private int $value = 0;

	public function __construct(private int $step = 1){}

	public function increment() : void{
		$this->value += $this->step;
	}

	public function get() : int{
		return $this->value;
	}

	/**
	 * @return \Generator<int, int, int, string>
	 */
	public function steps(int $n) : \Generator{
		$sum = 0;
		for($i = 1; $i <= $n; $i++){
			$sum += (yield $i * $this->step) ?? 0;
		}
		return "sum=$sum";
	}
}
