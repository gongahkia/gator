using DocumentFormat.OpenXml;
using DocumentFormat.OpenXml.Packaging;
using DocumentFormat.OpenXml.Validation;
using System.Text.Json;

var version = FileFormatVersions.Office2007;
if (args.Length > 0 && args[0] == "--office2019") { version = FileFormatVersions.Office2019; args = args[1..]; }
if (args.Length == 0)
{
    Console.Error.WriteLine("Usage: OfficeValidation [--office2019] file.docx|file.xlsx|file.pptx [...]");
    return 2;
}

var failed = false;
foreach (var path in args)
{
    try
    {
        using OpenXmlPackage package = Path.GetExtension(path).ToLowerInvariant() switch {
            ".docx" => WordprocessingDocument.Open(path, false),
            ".xlsx" => SpreadsheetDocument.Open(path, false),
            ".pptx" => PresentationDocument.Open(path, false),
            _ => throw new ArgumentException("Expected DOCX, XLSX or PPTX")
        };
        var issues = new OpenXmlValidator(version)
            .Validate(package).Select(issue => new {
                issue.Id, issue.Description,
                Part = issue.Part?.Uri.ToString(),
                Path = issue.Path?.XPath
            }).ToArray();
        Console.WriteLine(JsonSerializer.Serialize(new { File = path, Version = version.ToString(), Errors = issues.Length, Issues = issues }));
        failed |= issues.Length != 0;
    }
    catch (Exception error)
    {
        Console.Error.WriteLine($"{path}: {error.Message}");
        failed = true;
    }
}
return failed ? 1 : 0;
