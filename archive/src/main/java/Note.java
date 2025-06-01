import com.opencsv.bean.CsvBindByName;
import com.opencsv.bean.CsvDate;
import io.hypersistence.utils.hibernate.type.search.PostgreSQLTSVectorType;
import jakarta.persistence.*;
import lombok.Data;
import lombok.ToString;
import org.hibernate.annotations.GeneratedColumn;
import org.hibernate.annotations.Type;
import org.hibernate.dialect.PostgresPlusDialect;

import java.time.LocalDateTime;


/**
 * Represents a community note on a tweet.
 * <br>
 * The format of a note is described in <a href="https://communitynotes.x.com/guide/en/under-the-hood/download-data">this page</a>
 * <br>
 * This class is annotated to serve as both a Hibernate entity and a CSV bean for use with OpenCSV.
 */
@Entity
@Table(indexes = {
        @Index(columnList = "noteAuthorParticipantId"),
        @Index(columnList = "tweetId"),
        @Index(columnList = "createdAtMillis"),
})
@Data
@ToString(onlyExplicitlyIncluded = true)
public class Note {

    @ToString.Include
    @CsvBindByName
    @Id
    // should be a long but the value is too high so JSON parsing generates rounding effects
    String noteId;

    @CsvBindByName
    String noteAuthorParticipantId;

    @CsvBindByName
    Long createdAtMillis;

    @ToString.Include
    @CsvBindByName
    // should be a long but the value is too high so JSON parsing generates rounding effects
    String tweetId;

    @CsvBindByName
    String classification;

    @Deprecated
    @CsvBindByName
    String believable;

    @Deprecated
    @CsvBindByName
    String harmful;

    @Deprecated
    @CsvBindByName
    String validationDifficulty;

    @CsvBindByName
    int misleadingOther;

    @CsvBindByName
    int misleadingFactualError;

    @CsvBindByName
    int misleadingManipulatedMedia;

    @CsvBindByName
    int misleadingOutdatedInformation;

    @CsvBindByName
    int misleadingMissingImportantContext;

    @CsvBindByName
    int misleadingUnverifiedClaimAsFact;

    @CsvBindByName
    int misleadingSatire;

    @CsvBindByName
    int notMisleadingOther;

    @CsvBindByName
    int notMisleadingFactuallyCorrect;

    @CsvBindByName
    int notMisleadingOutdatedButNotWhenWritten;

    @CsvBindByName
    int notMisleadingClearlySatire;

    @CsvBindByName
    int notMisleadingPersonalOpinion;

    @CsvBindByName
    int trustworthySources;

    @ToString.Include
    @CsvBindByName
    // The size limit of a note is 280 characters but links are not counted, so the actual limit is not known
    // Currently 8192 is enough to load all notes
    @Column(length = 8192)
    String summary;

    @CsvBindByName
    int isMediaNote;

    /**
     * This field is used for full-text search.<br>
     * It will generate a column of type {@code tsvector}, which is computed from the {@code summary} column.
     * The column can then be used for full-text search using the @@ operator e.g.
     * {@code SELECT * FROM note WHERE summary_ts @@ to_tsquery('english', 'Nigeria')}
     * or with Postgrest using a Full-text search filter e.g. {@code ?summary_ts=fts.Nigeria},
     * see <a href="https://docs.postgrest.org/en/stable/references/api/tables_views.html#fts">Postgrest Full-Text Search operators</a>
     * <br>
     * This column should be indexed with a GIN index; unfortunately, Hibernate does not support non-standard indexes,
     * so the index has to be created manually like this: {@code CREATE INDEX IF NOT EXISTS ts_idx ON note USING GIN (summary_ts);}
     * <br>
     * Requires {@link PostgreSQLTSVectorType} from  <a href="https://github.com/vladmihalcea/hypersistence-utils">hypersistence-utils</a>
     * <br>
     * This was inspired by <a href="https://www.crunchydata.com/blog/postgres-full-text-search-a-search-engine-in-a-database">this article</a>
     */
    @GeneratedColumn("to_tsvector('english', summary)")
    @Type(PostgreSQLTSVectorType.class)
    @Column(columnDefinition = "tsvector")
    String summary_ts;

}
