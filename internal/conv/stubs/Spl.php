<?php

// phar2go's PHP versions of SPL classes used by plugins (converted with the plugin).

class SplObjectStorage implements \Countable, \Iterator, \ArrayAccess{
	private array $objects = [];
	private array $data = [];
	private int $position = 0;

	public function attach(object $object, mixed $info = null) : void{
		$id = spl_object_id($object);
		$this->objects[$id] = $object;
		$this->data[$id] = $info;
	}

	public function detach(object $object) : void{
		$id = spl_object_id($object);
		unset($this->objects[$id], $this->data[$id]);
	}

	public function contains(object $object) : bool{
		return isset($this->objects[spl_object_id($object)]);
	}

	public function count() : int{
		return count($this->objects);
	}

	public function getInfo() : mixed{
		$keys = array_keys($this->objects);
		return isset($keys[$this->position]) ? $this->data[$keys[$this->position]] : null;
	}

	public function setInfo(mixed $info) : void{
		$keys = array_keys($this->objects);
		if(isset($keys[$this->position])){
			$this->data[$keys[$this->position]] = $info;
		}
	}

	public function removeAll(SplObjectStorage $storage) : int{
		foreach($storage->objects as $object){
			$this->detach($object);
		}
		return count($this->objects);
	}

	public function addAll(SplObjectStorage $storage) : int{
		foreach($storage->objects as $id => $object){
			$this->objects[$id] = $object;
			$this->data[$id] = $storage->data[$id];
		}
		return count($this->objects);
	}

	public function offsetExists(mixed $offset) : bool{
		return $this->contains($offset);
	}

	public function offsetGet(mixed $offset) : mixed{
		return $this->data[spl_object_id($offset)] ?? null;
	}

	public function offsetSet(mixed $offset, mixed $value) : void{
		$this->attach($offset, $value);
	}

	public function offsetUnset(mixed $offset) : void{
		$this->detach($offset);
	}

	public function current() : mixed{
		$keys = array_keys($this->objects);
		return isset($keys[$this->position]) ? $this->objects[$keys[$this->position]] : null;
	}

	public function key() : mixed{
		return $this->position;
	}

	public function next() : void{
		$this->position++;
	}

	public function rewind() : void{
		$this->position = 0;
	}

	public function valid() : bool{
		return $this->position < count($this->objects);
	}
}

class ArrayObject implements \Countable, \IteratorAggregate, \ArrayAccess{
	private array $storage;

	public function __construct(array|object $array = []){
		$this->storage = (array) $array;
	}

	public function getArrayCopy() : array{
		return $this->storage;
	}

	public function count() : int{
		return count($this->storage);
	}

	public function append(mixed $value) : void{
		$this->storage[] = $value;
	}

	public function getIterator() : \Iterator{
		return new ArrayIterator($this->storage);
	}

	public function offsetExists(mixed $key) : bool{
		return isset($this->storage[$key]);
	}

	public function offsetGet(mixed $key) : mixed{
		return $this->storage[$key] ?? null;
	}

	public function offsetSet(mixed $key, mixed $value) : void{
		if($key === null){
			$this->storage[] = $value;
		}else{
			$this->storage[$key] = $value;
		}
	}

	public function offsetUnset(mixed $key) : void{
		unset($this->storage[$key]);
	}
}

class ArrayIterator implements \Countable, \Iterator, \ArrayAccess{
	private array $storage;
	private array $keys;
	private int $position = 0;

	public function __construct(array|object $array = []){
		$this->storage = (array) $array;
		$this->keys = array_keys($this->storage);
	}

	public function getArrayCopy() : array{
		return $this->storage;
	}

	public function count() : int{
		return count($this->storage);
	}

	public function current() : mixed{
		return $this->storage[$this->keys[$this->position]] ?? null;
	}

	public function key() : mixed{
		return $this->keys[$this->position] ?? null;
	}

	public function next() : void{
		$this->position++;
	}

	public function rewind() : void{
		$this->keys = array_keys($this->storage);
		$this->position = 0;
	}

	public function valid() : bool{
		return $this->position < count($this->keys);
	}

	public function offsetExists(mixed $key) : bool{
		return isset($this->storage[$key]);
	}

	public function offsetGet(mixed $key) : mixed{
		return $this->storage[$key] ?? null;
	}

	public function offsetSet(mixed $key, mixed $value) : void{
		if($key === null){
			$this->storage[] = $value;
		}else{
			$this->storage[$key] = $value;
		}
	}

	public function offsetUnset(mixed $key) : void{
		unset($this->storage[$key]);
	}
}

class SplDoublyLinkedList implements \Countable, \Iterator, \ArrayAccess{
	protected array $items = [];
	private int $position = 0;

	public function push(mixed $value) : void{
		$this->items[] = $value;
	}

	public function pop() : mixed{
		if(count($this->items) === 0){
			throw new \RuntimeException("Can't pop from an empty datastructure");
		}
		return array_pop($this->items);
	}

	public function shift() : mixed{
		if(count($this->items) === 0){
			throw new \RuntimeException("Can't shift from an empty datastructure");
		}
		return array_shift($this->items);
	}

	public function unshift(mixed $value) : void{
		array_unshift($this->items, $value);
	}

	public function top() : mixed{
		if(count($this->items) === 0){
			throw new \RuntimeException("Can't peek at an empty datastructure");
		}
		return $this->items[count($this->items) - 1];
	}

	public function bottom() : mixed{
		if(count($this->items) === 0){
			throw new \RuntimeException("Can't peek at an empty datastructure");
		}
		return $this->items[0];
	}

	public function isEmpty() : bool{
		return count($this->items) === 0;
	}

	public function count() : int{
		return count($this->items);
	}

	public function toArray() : array{
		return $this->items;
	}

	public function current() : mixed{
		return $this->items[$this->position] ?? null;
	}

	public function key() : mixed{
		return $this->position;
	}

	public function next() : void{
		$this->position++;
	}

	public function rewind() : void{
		$this->position = 0;
	}

	public function valid() : bool{
		return $this->position < count($this->items);
	}

	public function offsetExists($index) : bool{
		return isset($this->items[$index]);
	}

	public function offsetGet($index) : mixed{
		return $this->items[$index] ?? null;
	}

	public function offsetSet($index, mixed $value) : void{
		if($index === null){
			$this->items[] = $value;
		}else{
			$this->items[$index] = $value;
		}
	}

	public function offsetUnset($index) : void{
		unset($this->items[$index]);
		$this->items = array_values($this->items);
	}
}

class SplQueue extends SplDoublyLinkedList{
	public function enqueue(mixed $value) : void{
		$this->push($value);
	}

	public function dequeue() : mixed{
		return $this->shift();
	}
}

class SplStack extends SplDoublyLinkedList{
	public function current() : mixed{
		return $this->items[count($this->items) - 1 - $this->key()] ?? null;
	}
}

class SplFixedArray implements \Countable, \Iterator, \ArrayAccess{
	private array $items;
	private int $position = 0;

	public function __construct(int $size = 0){
		$this->items = $size > 0 ? array_fill(0, $size, null) : [];
	}

	public static function fromArray(array $array, bool $preserveKeys = true) : SplFixedArray{
		$fixed = new SplFixedArray(count($array));
		$i = 0;
		foreach($array as $k => $v){
			$fixed->items[$preserveKeys ? $k : $i++] = $v;
		}
		return $fixed;
	}

	public function getSize() : int{
		return count($this->items);
	}

	public function setSize(int $size) : bool{
		$this->items = array_slice(array_pad($this->items, $size, null), 0, $size);
		return true;
	}

	public function toArray() : array{
		return $this->items;
	}

	public function count() : int{
		return count($this->items);
	}

	public function current() : mixed{
		return $this->items[$this->position] ?? null;
	}

	public function key() : mixed{
		return $this->position;
	}

	public function next() : void{
		$this->position++;
	}

	public function rewind() : void{
		$this->position = 0;
	}

	public function valid() : bool{
		return $this->position < count($this->items);
	}

	public function offsetExists($index) : bool{
		return isset($this->items[$index]);
	}

	public function offsetGet($index) : mixed{
		return $this->items[$index] ?? null;
	}

	public function offsetSet($index, mixed $value) : void{
		$this->items[$index] = $value;
	}

	public function offsetUnset($index) : void{
		$this->items[$index] = null;
	}
}

class SplFileInfo{
	protected string $pathname;

	public function __construct(string $filename){
		$this->pathname = $filename;
	}

	public function getPathname() : string{
		return $this->pathname;
	}

	public function getFilename() : string{
		return basename($this->pathname);
	}

	public function getBasename(string $suffix = "") : string{
		return basename($this->pathname, $suffix);
	}

	public function getPath() : string{
		return dirname($this->pathname);
	}

	public function getExtension() : string{
		$name = $this->getFilename();
		$dot = strrpos($name, ".");
		return $dot === false ? "" : substr($name, $dot + 1);
	}

	public function getRealPath() : string|false{
		return realpath($this->pathname);
	}

	public function isDir() : bool{
		return is_dir($this->pathname);
	}

	public function isFile() : bool{
		return is_file($this->pathname);
	}

	public function getSize() : int|false{
		return filesize($this->pathname);
	}

	public function getMTime() : int|false{
		return filemtime($this->pathname);
	}

	public function __toString() : string{
		return $this->pathname;
	}
}

class FilesystemIterator implements \Iterator{
	public const CURRENT_AS_PATHNAME = 32;
	public const CURRENT_AS_FILEINFO = 0;
	public const CURRENT_AS_SELF = 16;
	public const KEY_AS_PATHNAME = 0;
	public const KEY_AS_FILENAME = 256;
	public const FOLLOW_SYMLINKS = 512;
	public const NEW_CURRENT_AND_KEY = 256;
	public const SKIP_DOTS = 4096;
	public const UNIX_PATHS = 8192;

	protected string $path;
	protected int $flags;
	protected array $entries = [];
	protected int $position = 0;

	public function __construct(string $directory, int $flags = self::KEY_AS_PATHNAME | self::CURRENT_AS_FILEINFO | self::SKIP_DOTS){
		$this->path = rtrim($directory, "/\\");
		$this->flags = $flags;
		$list = scandir($this->path);
		foreach($list === false ? [] : $list as $name){
			if(($name === "." || $name === "..") && ($flags & self::SKIP_DOTS) !== 0){
				continue;
			}
			$this->entries[] = $name;
		}
	}

	public function current() : mixed{
		$path = $this->path . "/" . $this->entries[$this->position];
		if(($this->flags & self::CURRENT_AS_PATHNAME) !== 0){
			return $path;
		}
		return new SplFileInfo($path);
	}

	public function key() : mixed{
		$name = $this->entries[$this->position];
		return ($this->flags & self::KEY_AS_FILENAME) !== 0 ? $name : $this->path . "/" . $name;
	}

	public function next() : void{
		$this->position++;
	}

	public function rewind() : void{
		$this->position = 0;
	}

	public function valid() : bool{
		return $this->position < count($this->entries);
	}

	public function hasChildren(bool $allowLinks = false) : bool{
		$name = $this->entries[$this->position] ?? ".";
		return $name !== "." && $name !== ".." && is_dir($this->path . "/" . $name);
	}

	public function getChildren() : RecursiveDirectoryIterator{
		return new RecursiveDirectoryIterator($this->path . "/" . $this->entries[$this->position], $this->flags);
	}
}

class DirectoryIterator extends FilesystemIterator{
	public function __construct(string $directory){
		parent::__construct($directory, 0);
	}
}

class RecursiveDirectoryIterator extends FilesystemIterator{
}

class RecursiveIteratorIterator implements \Iterator{
	public const LEAVES_ONLY = 0;
	public const SELF_FIRST = 1;
	public const CHILD_FIRST = 2;
	public const CATCH_GET_CHILD = 16;

	private array $items = [];
	private int $position = 0;

	public function __construct($iterator, int $mode = self::LEAVES_ONLY, int $flags = 0){
		$this->collect($iterator, $mode);
	}

	private function collect($iterator, int $mode) : void{
		foreach($iterator as $key => $value){
			$children = $iterator->hasChildren() ? $iterator->getChildren() : null;
			if($children !== null){
				if($mode === self::SELF_FIRST){
					$this->items[] = [$key, $value];
				}
				$this->collect($children, $mode);
				if($mode === self::CHILD_FIRST){
					$this->items[] = [$key, $value];
				}
			}else{
				$this->items[] = [$key, $value];
			}
		}
	}

	public function current() : mixed{
		return $this->items[$this->position][1] ?? null;
	}

	public function key() : mixed{
		return $this->items[$this->position][0] ?? null;
	}

	public function next() : void{
		$this->position++;
	}

	public function rewind() : void{
		$this->position = 0;
	}

	public function valid() : bool{
		return $this->position < count($this->items);
	}
}

class ReflectionClass{
	private string $name;

	public function __construct(object|string $objectOrClass){
		$this->name = is_object($objectOrClass) ? get_class($objectOrClass) : ltrim($objectOrClass, "\\");
	}

	public function getName() : string{
		return $this->name;
	}

	public function getShortName() : string{
		$pos = strrpos($this->name, "\\");
		return $pos === false ? $this->name : substr($this->name, $pos + 1);
	}

	public function getNamespaceName() : string{
		$pos = strrpos($this->name, "\\");
		return $pos === false ? "" : substr($this->name, 0, $pos);
	}

	public function inNamespace() : bool{
		return strrpos($this->name, "\\") !== false;
	}

	public function newInstance(mixed ...$args) : object{
		return phar2go_new($this->name, ...$args);
	}

	public function newInstanceArgs(array $args = []) : object{
		return phar2go_new($this->name, ...array_values($args));
	}

	public function isSubclassOf(string $class) : bool{
		return is_subclass_of($this->name, $class, true);
	}

	public function implementsInterface(string $interface) : bool{
		return is_subclass_of($this->name, $interface, true);
	}

	public function isInstance(object $object) : bool{
		return is_a($object, $this->name);
	}

	public function getParentClass() : ReflectionClass|false{
		$parent = get_parent_class($this->name);
		return $parent === false ? false : new ReflectionClass($parent);
	}

	public function getConstants() : array{
		return phar2go_class_constants($this->name);
	}

	public function getConstant(string $name) : mixed{
		$constants = phar2go_class_constants($this->name);
		return $constants[$name] ?? false;
	}

	public function hasConstant(string $name) : bool{
		return array_key_exists($name, phar2go_class_constants($this->name));
	}

	public function isAbstract() : bool{
		return false;
	}

	public function isInterface() : bool{
		return false;
	}

	public function isInstantiable() : bool{
		return class_exists($this->name);
	}
}

class WeakMap implements \Countable, \ArrayAccess, \IteratorAggregate{
	private SplObjectStorage $storage;

	public function __construct(){
		$this->storage = new SplObjectStorage();
	}

	public function count() : int{
		return $this->storage->count();
	}

	public function offsetExists($object) : bool{
		return $this->storage->contains($object);
	}

	public function offsetGet($object) : mixed{
		return $this->storage->offsetGet($object);
	}

	public function offsetSet($object, mixed $value) : void{
		$this->storage->attach($object, $value);
	}

	public function offsetUnset($object) : void{
		$this->storage->detach($object);
	}

	public function getIterator() : \Iterator{
		$pairs = [];
		foreach($this->storage as $i => $object){
			$pairs[] = [$object, $this->storage->offsetGet($object)];
		}
		return new ArrayIterator($pairs);
	}
}
