import React, { useEffect, useState, useId, useRef } from 'react';
import mermaid from 'mermaid';
import { Workflow, Code2, Copy, Check, AlertTriangle, RefreshCw } from 'lucide-react';

interface ArchitecturalDiagramProps {
  code: string;
}

export const ArchitecturalDiagram: React.FC<ArchitecturalDiagramProps> = ({ code }) => {
  const [svgHtml, setSvgHtml] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'diagram' | 'code'>('diagram');
  const [copied, setCopied] = useState<boolean>(false);
  const [isRendering, setIsRendering] = useState<boolean>(true);
  const containerRef = useRef<HTMLDivElement>(null);

  // Generate a valid element id for mermaid render
  const reactId = useId().replace(/[^a-zA-Z0-9_-]/g, '');
  const diagramId = `mermaid-${reactId}`;

  // Check if dark mode is active
  const isDarkMode = (): boolean => {
    return (
      document.documentElement.classList.contains('dark') ||
      window.matchMedia('(prefers-color-scheme: dark)').matches
    );
  };

  const renderDiagram = async () => {
    if (!code || !code.trim()) {
      setSvgHtml('');
      setError('Empty diagram definition');
      setIsRendering(false);
      return;
    }

    setIsRendering(true);
    setError(null);

    try {
      const dark = isDarkMode();
      mermaid.initialize({
        startOnLoad: false,
        theme: dark ? 'dark' : 'default',
        securityLevel: 'loose',
        fontFamily: 'inherit',
      });

      // mermaid.render needs a unique ID without special characters
      const cleanCode = code.trim();
      const renderId = `${diagramId}-${Date.now()}`;
      const { svg } = await mermaid.render(renderId, cleanCode);
      setSvgHtml(svg);
      setError(null);
    } catch (err: unknown) {
      console.warn('Mermaid rendering error:', err);
      const errMsg = err instanceof Error ? err.message : String(err);
      setError(errMsg);
      // Clean up any error SVG elements mermaid might have appended to document body
      const errorElements = document.querySelectorAll(`[id^="${diagramId}"]`);
      errorElements.forEach((el) => {
        if (el.parentNode && el !== containerRef.current) {
          el.parentNode.removeChild(el);
        }
      });
    } finally {
      setIsRendering(false);
    }
  };

  useEffect(() => {
    renderDiagram();

    // Listen for theme changes on html element
    const observer = new MutationObserver(() => {
      renderDiagram();
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    });

    return () => {
      observer.disconnect();
    };
  }, [code]);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (e) {
      console.error('Failed to copy diagram code', e);
    }
  };

  return (
    <div className="my-6 rounded-xl border border-border bg-card/60 backdrop-blur shadow-sm overflow-hidden not-prose">
      {/* Header Toolbar */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-muted/40 border-b border-border text-xs font-medium">
        <div className="flex items-center gap-2 text-foreground font-semibold">
          <Workflow className="h-4 w-4 text-primary" />
          <span>Architectural Diagram</span>
        </div>

        <div className="flex items-center gap-1.5">
          <div className="flex items-center bg-muted/80 rounded-lg p-0.5 border border-border">
            <button
              type="button"
              onClick={() => setViewMode('diagram')}
              className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition flex items-center gap-1 ${
                viewMode === 'diagram'
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Workflow className="h-3 w-3" />
              <span>Diagram</span>
            </button>
            <button
              type="button"
              onClick={() => setViewMode('code')}
              className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition flex items-center gap-1 ${
                viewMode === 'code'
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Code2 className="h-3 w-3" />
              <span>Source</span>
            </button>
          </div>

          <button
            type="button"
            onClick={handleCopy}
            title="Copy Mermaid Code"
            className="p-1.5 rounded-lg border border-border bg-background hover:bg-muted text-muted-foreground hover:text-foreground transition flex items-center gap-1"
          >
            {copied ? (
              <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
          </button>
        </div>
      </div>

      {/* Main Content Area */}
      <div className="p-4 overflow-x-auto min-h-[120px] flex items-center justify-center">
        {isRendering ? (
          <div className="py-8 flex items-center gap-2 text-xs text-muted-foreground">
            <RefreshCw className="h-4 w-4 animate-spin text-primary" />
            Rendering Architectural Diagram...
          </div>
        ) : error ? (
          <div className="w-full space-y-3">
            <div className="p-3 rounded-lg bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-900 text-amber-800 dark:text-amber-300 text-xs flex items-start gap-2.5">
              <AlertTriangle className="h-4 w-4 text-amber-600 shrink-0 mt-0.5" />
              <div className="space-y-1">
                <div className="font-semibold">Unable to render visual flowchart</div>
                <div className="text-[11px] opacity-90 font-mono break-all">{error}</div>
              </div>
            </div>
            <pre className="p-3 rounded-lg bg-muted/60 border border-border text-xs font-mono overflow-x-auto text-foreground whitespace-pre">
              {code}
            </pre>
          </div>
        ) : viewMode === 'diagram' ? (
          <div
            ref={containerRef}
            className="w-full flex justify-center py-2 [&>svg]:max-w-full [&>svg]:h-auto"
            dangerouslySetInnerHTML={{ __html: svgHtml }}
          />
        ) : (
          <pre className="w-full p-3 rounded-lg bg-muted/60 border border-border text-xs font-mono overflow-x-auto text-foreground whitespace-pre">
            {code}
          </pre>
        )}
      </div>
    </div>
  );
};
