package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) decodeRequest(w http.ResponseWriter, request *http.Request, destination any) error {
	limit := s.config.Server.MaxBodyBytes
	if limit <= 0 {
		limit = 16 << 20
	}
	request.Body = http.MaxBytesReader(w, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return errors.New("request contains multiple JSON values")
	}
	return nil
}
