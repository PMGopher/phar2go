<?php

namespace __NS__\exception;

use Exception;

class SQLException extends Exception
{
    public function __construct(
        protected array $_trace = [],

        protected string $_traceAsString = "",
        protected string $_message = "",
        protected string $_file = "",

        protected int $_code = 0,
        protected int $_line = 0,
    ) {
        parent::__construct($this->_message, $this->_code);
    }

    public function _getMessage(): string
    {
        return $this->_message;
    }

    public function _getCode(): int
    {
        return $this->_code;
    }

    public function _getTrace(): array
    {
        return $this->_trace;
    }

    public function _getFile(): string
    {
        return $this->_file;
    }

    public function _getLine(): int
    {
        return $this->_line;
    }

    public function _getTraceAsString(): string
    {
        return $this->_traceAsString;
    }

    public static function fromArray(array $exception): SQLException
    {
        $class = $exception["class"] ?? SQLException::class;

        return new $class(
            _trace: $exception["trace"],
            _traceAsString: $exception["trace_string"],

            _message: $exception["message"],

            _file: $exception["file"],
            _code: $exception["code"],
            _line: $exception["line"],
        );
    }
}
