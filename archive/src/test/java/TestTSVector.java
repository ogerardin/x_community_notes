import lombok.extern.slf4j.Slf4j;
import org.hibernate.boot.MetadataSources;
import org.hibernate.boot.registry.StandardServiceRegistryBuilder;

import java.util.concurrent.atomic.AtomicInteger;

//@Slf4j
public class TestTSVector {

    public static void main(String[] args) {
//        log.info("Configuring Hibernate");
        var registry = new StandardServiceRegistryBuilder().build();
        try (var sessionFactory = new MetadataSources(registry)
                .addAnnotatedClass(Note.class)
                .buildMetadata()
                .buildSessionFactory()) {

//            log.info("Saving notes to database");
            sessionFactory.inSession(session -> session.doWork(
                    connection -> {
                        connection.createStatement().execute(
                            "CREATE INDEX IF NOT EXISTS ts_idx ON note USING GIN (summary_ts);"
                        );
                    }
                )
            );
            sessionFactory.inStatelessSession(session -> {
//                    log.info("Saving note");
                    Note note = new Note();
                    note.setNoteId("1234567890");
                    note.setSummary("Test summary");
                    session.insert(note);
                }
            );
        }
    }

}
