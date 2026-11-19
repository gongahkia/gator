// read-only external sampler for one supplied disposable process; no rendering
#include <libproc.h>
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include <unistd.h>
int main(int argc,char **argv){
 if(argc!=3)return 2;
 int pid=atoi(argv[1]),n=atoi(argv[2]);if(pid<2||n<1||n>300)return 2;
 for(int i=0;i<n;i++){
  struct rusage_info_v4 info={0};if(proc_pid_rusage(pid,RUSAGE_INFO_V4,(rusage_info_t*)&info))return 1;
  struct timespec t;clock_gettime(CLOCK_REALTIME,&t);
  printf("{\"epoch\":%.6f,\"rss\":%llu,\"footprint\":%llu,\"peakFootprint\":%llu,\"cpuNanoseconds\":%llu}\n",(double)t.tv_sec+t.tv_nsec/1e9,info.ri_resident_size,info.ri_phys_footprint,info.ri_lifetime_max_phys_footprint,info.ri_user_time+info.ri_system_time);fflush(stdout);
  struct timespec interval={.tv_sec=1};nanosleep(&interval,NULL);
 }
 return 0;
}
