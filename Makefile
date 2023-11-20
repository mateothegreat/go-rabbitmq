test/setup:
	docker compose -f test/docker-compose.yaml up -d

test/teardown:
	docker compose -f test/docker-compose.yaml down

test/reset: test/teardown test/setup
