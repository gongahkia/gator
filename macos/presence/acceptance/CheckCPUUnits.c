// calibrate the read-only sampler against POSIX process CPU time on this host.
#include <libproc.h>
#include <mach/mach_time.h>
#include <stdio.h>
#include <sys/resource.h>
#include <time.h>
#include <unistd.h>
static double seconds(struct rusage *r) {return r->ru_utime.tv_sec+r->ru_utime.tv_usec/1e6+r->ru_stime.tv_sec+r->ru_stime.tv_usec/1e6;}
int main(void) {
 struct rusage_info_v4 before={0},after={0};struct rusage a,b;
 mach_timebase_info_data_t scale;mach_timebase_info(&scale);
 proc_pid_rusage(getpid(),RUSAGE_INFO_V4,(rusage_info_t*)&before);getrusage(RUSAGE_SELF,&a);
 struct timespec start,now;clock_gettime(CLOCK_MONOTONIC,&start);volatile unsigned long value=1;
 do {for(int i=0;i<10000;i++)value=value*1664525+1013904223;clock_gettime(CLOCK_MONOTONIC,&now);} while((now.tv_sec-start.tv_sec)+(now.tv_nsec-start.tv_nsec)/1e9<0.25);
 getrusage(RUSAGE_SELF,&b);proc_pid_rusage(getpid(),RUSAGE_INFO_V4,(rusage_info_t*)&after);
 unsigned long long delta=after.ri_user_time+after.ri_system_time-before.ri_user_time-before.ri_system_time;
 printf("{\"POSIXCPUSeconds\":%.9f,\"rusageRawDelta\":%llu,\"rawDividedBy1e9\":%.9f,\"rawWithMachTimebaseSeconds\":%.9f,\"machNumer\":%u,\"machDenom\":%u}\n",seconds(&b)-seconds(&a),delta,delta/1e9,delta/1e9*scale.numer/scale.denom,scale.numer,scale.denom);
}
