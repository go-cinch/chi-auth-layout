package rpc

import (
 "context"
 "errors"
 "fmt"
 "log/slog"
 "runtime/debug"
 "time"

 "{{ .Computed.module_name_final }}/internal/common/config"
 "google.golang.org/grpc"
 "google.golang.org/grpc/codes"
 "google.golang.org/grpc/peer"
 "google.golang.org/grpc/status"
)

func Unary(cfg *config.Config) grpc.UnaryServerInterceptor {
 return func(ctx context.Context,request any,info *grpc.UnaryServerInfo,next grpc.UnaryHandler)(response any,err error){
  err=invoke(ctx,cfg,info.FullMethod,cfg.GRPC.Timeout,func(ctx context.Context)error{
   response,err=next(ctx,request);return err
  })
  return response,err
 }
}

func Stream(cfg *config.Config) grpc.StreamServerInterceptor {
 return func(service any,stream grpc.ServerStream,info *grpc.StreamServerInfo,next grpc.StreamHandler)error{
  // Streams use the caller's deadline.
  return invoke(stream.Context(),cfg,info.FullMethod,0,func(ctx context.Context)error{
   return next(service,&contextStream{ServerStream:stream,ctx:ctx})
  })
 }
}

type contextStream struct {grpc.ServerStream;ctx context.Context}

func (s *contextStream) Context() context.Context {return s.ctx}

func invoke(ctx context.Context,cfg *config.Config,method string,timeout time.Duration,next func(context.Context)error)(err error){
 started:=time.Now()
 if timeout>0 {var cancel context.CancelFunc;ctx,cancel=context.WithTimeout(ctx,timeout);defer cancel()}
{{- if .Computed.enable_trace_final }}
 ctx,endTrace:=startTrace(ctx,cfg.Server.Name,method)
 defer func(){endTrace(err)}()
{{- end }}
 defer func(){
  if recovered:=recover();recovered!=nil {
   slog.ErrorContext(ctx,"panic recovered: "+fmt.Sprint(recovered),"stack",string(debug.Stack()))
   err=status.Error(codes.Internal,"internal server error")
  }
  if err==nil && ctx.Err()!=nil {err=ctx.Err()}
  if errors.Is(err,context.Canceled)||errors.Is(err,context.DeadlineExceeded) {err=status.FromContextError(err).Err()}
  address:=""
  if remote,ok:=peer.FromContext(ctx);ok && remote.Addr!=nil {address=remote.Addr.String()}
  slog.InfoContext(ctx,fmt.Sprintf("%s %s %dms",method,status.Code(err),time.Since(started).Milliseconds()),"remote_addr",address)
 }()
 return next(ctx)
}
