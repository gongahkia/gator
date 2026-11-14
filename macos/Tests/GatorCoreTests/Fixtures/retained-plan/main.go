// Controlled test setup only: use Go's actual Store API, never hand-edit approval JSON.
package main

import (
 "fmt"
 "os"
 "github.com/gongahkia/gator/internal/worksession"
)
func main() {
 if len(os.Args)!=4 { panic("disposable state directory, conversation, task required") }
 store,err:=worksession.Open(os.Args[1]);if err!=nil {panic(err)}
 journal,err:=store.LoadRequests(os.Args[2]);if err!=nil {panic(err)}
 for _,request:=range journal.Requests {
  if request.ID==os.Args[3] && request.Plan!=nil {
   _,changed,err:=store.ApproveTaskPlan(os.Args[2],request.ID,request.Plan.Revision,request.Plan.Digest)
   if err!=nil {panic(err)};fmt.Println("retained approved without dispatch",changed);return
  }
 }
 panic("controlled plan missing")
}
