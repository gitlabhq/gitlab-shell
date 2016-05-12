REDIS_RB_VERSION=v3.3.0
REDIS_RB_TMP_DIR := $(shell mktemp -d)
REDIS_RB_VENDOR_DIR=lib/vendor
PWD=`pwd`

all:

update-redis:
	git clone https://github.com/redis/redis-rb.git $(REDIS_RB_TMP_DIR)
	cd $(REDIS_RB_TMP_DIR); git checkout $(REDIS_RB_VERSION)
	cd $(PWD)
	mkdir -p $(REDIS_RB_VENDOR_DIR)
	cp -r $(REDIS_RB_TMP_DIR)/lib/* $(REDIS_RB_VENDOR_DIR)
	# Adjust all 'require redis/' paths to relative paths
	sed -i.orig -e 's/require "redis/require_relative "redis/g' $(REDIS_RB_VENDOR_DIR)/redis.rb
	find $(REDIS_RB_VENDOR_DIR)/redis -name \*.rb -maxdepth 1 -exec sed -i.orig -e "s/require \"redis\//require_relative \"/g" {} \;
	find $(REDIS_RB_VENDOR_DIR)/redis/connection -name \*.rb -maxdepth 1 -exec sed -i.orig -e 's/require "redis\/connection\//require_relative "/g' *.rb {} \;
	find $(REDIS_RB_VENDOR_DIR)/redis/connection -name \*.rb -maxdepth 1 -exec sed -i.orig -e 's/require "redis\//require_relative "..\//g' *.rb {} \;

.PHONY=update-redis
