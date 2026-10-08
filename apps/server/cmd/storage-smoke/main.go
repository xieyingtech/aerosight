// storage-smoke verifies configured storage using a small isolated object.
package main

import (
 "bytes"
 "context"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "time"

 "aerosight/server/internal/media"
 "github.com/google/uuid"
 "github.com/minio/minio-go/v7"
)

func main(){
 if err:=run();err!=nil{ // Provider errors can contain URLs; emit only the code.
  response:=minio.ToErrorResponse(err);code:=response.Code;if code==""{code="STORAGE_CHECK_FAILED"};fmt.Fprintln(os.Stderr,code);os.Exit(1)
 }
}
func run()error{
 cfg,err:=media.LoadS3Config();if err!=nil{return err}
 store,err:=media.NewConfiguredObjectStorage(os.Getenv("DATA_DIR"),cfg);if err!=nil{return err}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 body:=[]byte(`{"purpose":"aerosight-storage-acceptance"}`)
 key:="projects/1/acceptance/"+uuid.NewString()+".json"
 written,err:=store.PutObject(ctx,key,bytes.NewReader(body),"application/json");if err!=nil{return err}
 loaded,err:=store.GetObject(ctx,key);if err!=nil{return err}
 if !bytes.Equal(body,loaded.Body)||loaded.ChecksumSHA256!=written.ChecksumSHA256{return fmt.Errorf("content mismatch")}
 reader,err:=media.OpenStoredProjectObject(ctx,store,os.Getenv("DATA_DIR"),1,key);if err!=nil{return err};defer reader.Close()
 if _,err=reader.Seek(2,io.SeekStart);err!=nil{return err};part:=make([]byte,8);if _,err=io.ReadFull(reader,part);err!=nil{return err}
 if !bytes.Equal(part,body[2:10]){return fmt.Errorf("range mismatch")}
 return json.NewEncoder(os.Stdout).Encode(map[string]any{"passed":true,"bytes":len(body),"checksumSha256":written.ChecksumSHA256,"rangeRead":true,"objectKey":key})
}
