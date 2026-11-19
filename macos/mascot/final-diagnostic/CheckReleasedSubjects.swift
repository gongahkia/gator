import AppKit
import RealityKit
import Metal

@MainActor final class WeakSubject {
    weak var object: AnyObject?
    let id: String
    init(_ object: AnyObject){self.object=object;id=String(describing:ObjectIdentifier(object))}
    var alive: Bool{object != nil}
}

@main @MainActor struct CheckReleasedSubjects {
    struct Result{
        let wrapper:WeakSubject
        let target:WeakSubject
        let propertyCleared:Bool
    }
    static func one(cache:URL,pixels:Int,audit:LifetimeProbe) async throws -> Result{
        let template=try await NativeMeshArchive.read(cache).entity()
        guard let device=MTLCreateSystemDefaultDevice() else{throw NSError(domain:"NoMetal",code:1)}
        let engine=try Gate2Engine(template:template,device:device,configuration:EngineConfiguration(pixels:pixels,offscreen:true),audit:audit)
        try await engine.draw()
        func target(_ engine:Gate2Engine)->AnyObject?{
            guard let field=Mirror(reflecting:engine).children.first(where:{$0.label=="testTarget"}) else{return nil}
            return Mirror(reflecting:field.value).children.first?.value as AnyObject?
        }
        guard let weakTarget=target(engine).map(WeakSubject.init) else{throw NSError(domain:"TargetProbeUnavailable",code:1)}
        let weakEngine=WeakSubject(engine)
        await engine.close()
        return Result(wrapper:weakEngine,target:weakTarget,propertyCleared:target(engine)==nil)
    }
    static func main() async throws{
        let cache=URL(fileURLWithPath:CommandLine.arguments[1]);let audit=LifetimeProbe()
        var samples:[[String:Any]]=[];var subjects:[Result]=[];var assertions=0
        for index in 0..<12{
            var task:Task<Result,any Error>?=Task{try await one(cache:cache,pixels:index==11 ? 16:96,audit:audit)}
            let weak=try await task!.value;task=nil;subjects.append(weak)
            guard weak.propertyCleared,!weak.wrapper.alive,audit.live["RealityRenderer",default:0]==0 else{throw NSError(domain:"TestSubjectStillOwned",code:index)}
            assertions+=1
            samples.append(["cycle":index,"pixels":index==11 ? 16:96,"ownedTask":task != nil,
                            "aliveTargetsByGeneration":subjects.map{$0.target.alive},"live":audit.live])
        }
        let live=subjects.filter{$0.target.alive}.count
        guard live<=1,subjects.dropLast().allSatisfy({!$0.target.alive}) else{throw NSError(domain:"UnboundedTargetRetention",code:live)}
        assertions+=2
        let result:[String:Any]=["assertions":assertions,"samples":samples,"live":audit.live,
                                "all96PixelTargetsReleased":subjects.dropLast().allSatisfy{!$0.target.alive},
                                "last16PixelTargetAlive":subjects.last?.target.alive ?? false,
                                "applicationStrongSubjectReferences":0,"testInstrumentation":"weak probes; mirror reads only private testTarget presence; never retains subject between samples"]
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject:result,options:[.prettyPrinted,.sortedKeys]))
    }
}
