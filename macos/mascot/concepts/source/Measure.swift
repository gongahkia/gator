// Bounded offline renderer measurements, not an app/battery benchmark.
import Foundation
import Darwin
func cpuTime()->Double{var r=rusage();getrusage(RUSAGE_SELF,&r);return Double(r.ru_utime.tv_sec+r.ru_stime.tv_sec)+Double(r.ru_utime.tv_usec+r.ru_stime.tv_usec)/1_000_000}
@main struct Measure {
    static func main()throws{
        let stage=Stage(directions[1]);var rows:[[String:Any]]=[]
        for size in [96,256]{
            for _ in 0..<3{_ = stage.image(size:size)}
            let wall=ContinuousClock.now,cpu=cpuTime()
            for i in 0..<120 {autoreleasepool{stage.character.pose(open:CGFloat(i%20)/30,blink:0);_ = stage.image(size:size)}}
            let elapsed=wall.duration(to:.now),parts=elapsed.components
            rows.append(["pixels":size,"frames":120,"wall_seconds":Double(parts.seconds)+Double(parts.attoseconds)/1e18,"process_cpu_seconds":cpuTime()-cpu])
        }
        let before=cpuTime(),wall=ContinuousClock.now
        // No scene mutation or rendering during this explicit four-second
        // offline hold. This is not a measured on-screen RealityView idle.
        Thread.sleep(forTimeInterval:4)
        let data:[String:Any]=["host":"Apple M3 10-core GPU / macOS 26.5.2 / Swift 6.3.3","renderer":"SCNRenderer / Metal / 4x MSAA","results":rows,"offline_no_render_hold_cpu_seconds":cpuTime()-before,"offline_hold_wall":String(describing:wall.duration(to:.now)),"GPU_time":"not measured","battery":"not measured","RealityView_idle":"not measured"]
        try JSONSerialization.data(withJSONObject:data,options:[.prettyPrinted,.sortedKeys]).write(to:URL(fileURLWithPath:CommandLine.arguments[1]));print("Wrote bounded offline measurements")
    }
}
