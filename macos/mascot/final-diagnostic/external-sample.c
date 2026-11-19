// independent read-only sampling of one explicitly supplied fixture process
#include <errno.h>
#include <libproc.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/resource.h>
#include <time.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if (argc != 2) return 2;
    char *end;
    long target = strtol(argv[1], &end, 10);
    if (*end || target <= 1 || target > 2147483647) return 2;
    for (int sample = 0; sample < 2400; sample++) {
        struct rusage_info_v4 info = {0};
        if (proc_pid_rusage((int)target, RUSAGE_INFO_V4, (rusage_info_t *)&info) != 0) {
            if (errno == ESRCH) return 0;
            perror("proc_pid_rusage"); return 1;
        }
        struct timespec now;
        clock_gettime(CLOCK_REALTIME, &now);
        printf("{\"pid\":%ld,\"epochSeconds\":%.6f,\"rssBytes\":%llu,\"footprintBytes\":%llu,\"lifetimePeakFootprintBytes\":%llu}\n",
               target, (double)now.tv_sec + now.tv_nsec / 1e9,
               info.ri_resident_size, info.ri_phys_footprint, info.ri_lifetime_max_phys_footprint);
        fflush(stdout);
        struct timespec interval = {.tv_sec = 0, .tv_nsec = 50000000};
        nanosleep(&interval, NULL);
    }
    return 0;
}
