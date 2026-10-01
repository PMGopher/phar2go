<?php

namespace pmmp\thread;

/**
 * pmmp/pthreads' thread-safe objects. Go has no PHP threads: they are plain objects here.
 */
class ThreadSafe{
	public function synchronized(\Closure $function, mixed ...$args) : mixed{
		return $function(...$args);
	}

	public function notify() : bool{
		return true;
	}

	public function notifyOne() : bool{
		return true;
	}

	public function wait(int $timeout = 0) : bool{
		return true;
	}
}

final class ThreadSafeArray extends ThreadSafe implements \IteratorAggregate, \ArrayAccess, \Countable{
	private array $data = [];

	public static function fromArray(array $array) : ThreadSafeArray{
		$result = new ThreadSafeArray();
		$result->data = $array;
		return $result;
	}

	public function offsetExists(mixed $offset) : bool{
		return isset($this->data[$offset]);
	}

	public function offsetGet(mixed $offset) : mixed{
		return $this->data[$offset] ?? null;
	}

	public function offsetSet(mixed $offset, mixed $value) : void{
		if($offset === null){
			$this->data[] = $value;
		}else{
			$this->data[$offset] = $value;
		}
	}

	public function offsetUnset(mixed $offset) : void{
		unset($this->data[$offset]);
	}

	public function count() : int{
		return count($this->data);
	}

	public function getIterator() : \Iterator{
		return new \ArrayIterator($this->data);
	}

	public function shift() : mixed{
		return array_shift($this->data);
	}

	public function pop() : mixed{
		return array_pop($this->data);
	}

	public function chunk(int $size, bool $preserve = false) : array{
		$result = array_slice($this->data, 0, $size, $preserve);
		$this->data = array_slice($this->data, $size, null, $preserve);
		return $result;
	}

	public function merge(mixed $from, bool $overwrite = true) : bool{
		foreach($from as $k => $v){
			if($overwrite || !isset($this->data[$k])){
				$this->data[$k] = $v;
			}
		}
		return true;
	}

	public function toArray() : array{
		return $this->data;
	}
}
