#!/bin/bash

aprotoc --go_out=paths=source_relative:. symbols_map.proto

