test/setup:
	docker compose -f docker-compose.yaml up -d

test/teardown:
	docker compose -f docker-compose.yaml down

test/reset: test/teardown test/setup
