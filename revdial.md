To initialize it:
connman := connman.New()
then when devices connect:
func (s *Service) initiateDeviceConnection(w http.ResponseWriter, r *http.Request) {
	device := getDeviceFromContext(r.Context())
	project := getProjectFromContext(r.Context())
	if device == nil && project == nil {
		s.log.Error("get project or device")
		errutil.WriteCloudError(w, errutil.NewCloudError(http.StatusBadRequest, errutil.CloudErrorCodeInvalidParameter, "missing parameters"))
		return
	}

	s.withHijackedWebSocketConnection(w, r, func(clientConn net.Conn) {
		s.connman.Set(project.ID+device.ID, clientConn)
	})
}
you add them into your map. Then when you want to connect back the machine it's just:
deviceConn, err := s.connman.Dial(r.Context(), project.ID+device.ID)
	if err != nil {
		errutil.WriteCloudError(w, errutil.NewCloudError(http.StatusInternalServerError, errutil.CloudErrorCodeDeviceConnectionFailure, "failed to connect to device"))
		return
	}
	defer deviceConn.Close()
On agent side to initialize similar connection:
func (s *Service) serveRemote(ctx context.Context) error {
	conn, err := s.client.InitiateDeviceConnection(ctx)
	if err != nil {
		return errors.Wrap(err, "initiate connection")
	}

	listener := revdial.NewListener(conn, s.revdial)
	defer listener.Close()

	return s.remoteServer.Serve(listener)
}

func (s *Service) serveLocal(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.AgentURI)
	if err != nil {
		return err
	}
	defer listener.Close()

	return s.localServer.Serve(listener)
}

func (s *Service) revdial(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	conn, resp, err := s.client.Revdial(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	return conn.Conn, resp.Response, nil
}
where rev dial is:
func (c *client) Revdial(ctx context.Context, path string) (*websocket.Conn, *httputil.Response, error) {
	return websocket.DefaultDialer.Dial(
		ctx,
		getWebsocketURL(c.url, strings.TrimPrefix(path, "/")),
		nil,
	)
}
