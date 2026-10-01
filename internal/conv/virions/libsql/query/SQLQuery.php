<?php

namespace __NS__\query;

use Closure;
use __NS__\ConnectionPool;

abstract class SQLQuery
{
    protected string $identifier = "";
    protected ?string $error = null;
    protected mixed $result = null;

    public function getIdentifier(): string
    {
        return $this->identifier;
    }

    public function setIdentifier(string $identifier): void
    {
        $this->identifier = $identifier;
    }

    final public function getResult(): mixed
    {
        return $this->result;
    }

    final protected function setResult(mixed $result): void
    {
        $this->result = $result;
    }

    final public function getError(): ?string
    {
        return $this->error;
    }

    final public function execute(?Closure $onSuccess = null, ?Closure $onFail = null): void
    {
        ConnectionPool::getInstance()->submit($this, $onSuccess, $onFail);
    }
}
