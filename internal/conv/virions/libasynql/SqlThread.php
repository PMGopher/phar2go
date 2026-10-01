<?php

namespace __NS__;

interface SqlThread{
	public const MODE_GENERIC = 0;
	public const MODE_CHANGE = 1;
	public const MODE_INSERT = 2;
	public const MODE_SELECT = 3;
}
