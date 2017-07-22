REDIS_RB_VERSION=v3.3.3
REDIS_RB_VENDOR_DIR=lib/vendor/redis
PWD=`pwd`

all:

update-redis:
	rm -rf $(REDIS_RB_VENDOR_DIR)
	git clone -b $(REDIS_RB_VERSION) https://github.com/redis/redis-rb.git $(REDIS_RB_VENDOR_DIR)
	rm -rf $(REDIS_RB_VENDOR_DIR)/.git

.PHONY=update-redis
