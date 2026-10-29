using DocumentFormat.OpenXml;
using DocumentFormat.OpenXml.Packaging;
using DocumentFormat.OpenXml.Validation;
using System.Text.Json;

if (args.Length == 0)
{
    Console.Error.WriteLine("Usage: OfficeValidation deck.pptx [deck.pptx ...]");
    return 2;
}

var failed = false;
foreach (var path in args)
{
    try
    {
        using var deck = PresentationDocument.Open(path, false);
        var issues = new OpenXmlValidator(FileFormatVersions.Office2007)
            .Validate(deck).Select(issue => new {
                issue.Id, issue.Description,
                Part = issue.Part?.Uri.ToString(),
                Path = issue.Path?.XPath
            }).ToArray();
        Console.WriteLine(JsonSerializer.Serialize(new { File = path, Errors = issues.Length, Issues = issues }));
        failed |= issues.Length != 0;
    }
    catch (Exception error)
    {
        Console.Error.WriteLine($"{path}: {error.Message}");
        failed = true;
    }
}
return failed ? 1 : 0;
