up:
	docker-compose -p mono up -d

down:
	docker-compose -p mono down -v

multi.up:
	docker-compose -f docker-compose.multi.yml -p multi up -d

multi.down:
	docker-compose -f docker-compose.multi.yml -p multi down -v

admin:
	go run ./cmd/admin

schedules:
	go run ./cmd/schedules

sqlite:
	go run ./cmd/sqlite

tombstones:
	go run ./cmd/schedules
