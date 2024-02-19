BACKEND_TARGET=backend/message_passing.pb.go
FRONTEND_TARGET=frontend/src/message_passing.ts
all: $(FRONTEND_TARGET) $(BACKEND_TARGET) 

$(BACKEND_TARGET): message_passing.proto
	protoc --proto_path=. --go_out=backend/ --go_opt=paths=source_relative message_passing.proto

$(FRONTEND_TARGET): message_passing.proto
	protoc -I=. --ts_out=frontend/src message_passing.proto
