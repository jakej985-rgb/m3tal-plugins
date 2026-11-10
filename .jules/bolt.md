# Bolt's Journal

## 2026-11-05 - Avoid []rune Slice Conversions in Document Tokenization & Pre-allocate strings.Builder
**Learning:** In document indexing pipelines (such as `termCounts` and `splitIdentifiers` in `m3tal-ai`), converting complete document texts into `[]rune(text)` creates an intermediate slice allocating 4 bytes per rune on the heap. Combined with default un-sized `strings.Builder` growth, processing a ~100KB document incurred ~1.27 MB in allocations across 26 heap allocation cycles.
**Action:** Use native `for _, rune := range text` range loops over UTF-8 strings tracking previous rune state, and pre-size `strings.Builder` capacity with `expanded.Grow(len(text) + margin)` to achieve a single heap allocation and ~88% reduction in allocated memory.

## 2026-11-05 - Avoid Repeated Full-Document Feature Extraction in Linear Rule Checks
**Learning:** In document retrieval and metadata pipelines (such as `AssessFreshness` in `m3tal-ai`), checking text against a fixed list of sensitive terms by repeatedly invoking general-purpose tokenizers (like `features(text)` which generates both unigrams and bigrams) causes O(T * N) string allocations and re-parsing for every term. For a ~1,000-word document, this resulted in 30,000+ heap allocations (~5.2 MB) per assessment.
**Action:** When validating documents against a static dictionary of terms, precompute a hash set (`map[string]bool`) at package init and perform a single-pass tokenization pass with early return on the first match.

## 2026-11-10 - Order Row-Major Matrix Multiplication Loops (i, k, j) for Contiguous Cache Line Streaming
**Learning:** In dense neural network layers and linear algebra primitives (`Matrix.MatMul`), iterating in classic mathematical order `(row, col, k)` traverses `other.data` with stride `other.cols` in the innermost loop. This thrashes CPU L1/L2 cache lines because every inner step touches a different cache line.
**Action:** Reorder loops to `(row, k, col)` and hoist row offsets. Keeping scalar `aik = m.data[row*mCols + k]` in a register while letting the inner `col` loop stream contiguously through `data[rowCOffset+col]` and `other.data[rowBOffset+col]` enables hardware auto-vectorization and prefetching, speeding up dense matrix multiplication by ~2.4x to ~2.5x with zero memory allocation overhead.
