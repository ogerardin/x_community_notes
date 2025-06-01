import com.opencsv.bean.CsvToBeanBuilder;
import lombok.RequiredArgsConstructor;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import lukfor.progress.Components;
import lukfor.progress.TaskService;
import lukfor.progress.tasks.DownloadTask;
import net.lingala.zip4j.ZipFile;
import net.lingala.zip4j.exception.ZipException;
import net.lingala.zip4j.progress.ProgressMonitor;
import net.lingala.zip4j.progress.ProgressMonitor.Result;
import org.apache.commons.lang3.StringUtils;
import org.hibernate.boot.MetadataSources;
import org.hibernate.boot.registry.StandardServiceRegistryBuilder;

import java.io.FileNotFoundException;
import java.io.FileReader;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.time.LocalDate;
import java.util.concurrent.atomic.AtomicInteger;

import static lukfor.progress.Components.*;

@Slf4j
@RequiredArgsConstructor
public class Grabber implements Runnable {

    {
        boolean isTerminal = StringUtils.isNotBlank(System.getenv("TERM"));
        TaskService.setAnimated(isTerminal);
    }

    private final boolean forceDownload = false;
    private final Path workdir;


    @SneakyThrows
    public Path getWorkdir() {
        Files.createDirectories(workdir);
        return workdir;
    }

    @SneakyThrows
    public void run() {
        log.info("Downloading community notes file...");
        Path zippedNotesFile = getNotesFile();

        log.info("Unzipping community notes file...");
        Path tsvNotesFile = unzip(zippedNotesFile);

        log.info("Saving notes to database...");
        loadTsvFile(tsvNotesFile);
    }

    private Path getNotesFile() throws IOException {
        LocalDate today = LocalDate.now();
        try {
            log.debug("Trying notes file for today: {}", today);
            return downloadZippedNotesFile(today);
        } catch (Exception e) {
            log.debug("Failed to download notes file for today: {}", e.toString());
            LocalDate yesterday = today.minusDays(1);
            log.debug("Trying notes file for yesterday: {}", yesterday);
            return downloadZippedNotesFile(yesterday);
        }
    }


    private void loadTsvFile(Path tsvNotesFile) throws FileNotFoundException {
        log.debug("Configuring CSV parser for TSV file {}", tsvNotesFile);
        FileReader reader = new FileReader(tsvNotesFile.toFile());
        var csvParser = new CsvToBeanBuilder<Note>(reader)
                .withOrderedResults(false)
                .withSeparator('\t')
                .withType(Note.class)
                .build();

        log.debug("Configuring Hibernate");
        var registry = new StandardServiceRegistryBuilder().build();
        try (var sessionFactory = new MetadataSources(registry)
                .addAnnotatedClass(Note.class)
                .buildMetadata()
                .buildSessionFactory()) {

            log.debug("Creating GIN index on summary_ts");
            sessionFactory.inSession(session -> session.doWork(
                            connection -> {
                                connection.createStatement().execute("CREATE INDEX IF NOT EXISTS ts_idx ON note USING GIN (summary_ts);");
                            }
                    )
            );

            log.debug("Starting notes insertion");
            TaskService.monitor(SPINNER, TIME, TASK_NAME).run(monitor -> {
                monitor.begin("Saving notes...");
                AtomicInteger count = new AtomicInteger();
                sessionFactory.inStatelessSession(session -> {
                            csvParser.forEach(note -> {
                                monitor.update("Saving note #" + count.getAndIncrement());
                                session.insert(note);
                                monitor.worked(1);
                            });
                        }
                );
                monitor.update("Saved " + count.get() + " notes");
                monitor.done();
            });
        }
    }

    private Path unzip(Path notesZipFile) throws Exception {
        String notesFileName = "notes-00000.tsv";

        String tsvFileName = notesZipFile.getFileName().toString().replace(".zip", ".tsv");
        Path tsvFile = notesZipFile.resolveSibling(tsvFileName);

        log.debug("Extracting {} as {}...", notesFileName, tsvFile);
        try (ZipFile zipFile = new ZipFile(notesZipFile.toFile())) {
            ProgressMonitor zipProgressMonitor = zipFile.getProgressMonitor();
            zipFile.setRunInThread(true);
            TaskService.monitor(DEFAULT).run(monitor -> {
                monitor.begin("Extracting notes file", 100);
                try {
                    zipFile.extractFile(notesFileName, tsvFile.getParent().toString(), tsvFile.getFileName().toString());
                } catch (ZipException e) {
                    // must update the progress monitor manually because the exception is thrown before the monitor is updated
                    zipProgressMonitor.endProgressMonitor(e);
                    throw e;
                }

                int percent = 0;
                while (!zipProgressMonitor.getState().equals(ProgressMonitor.State.READY)) {
                    int zipPercent = zipProgressMonitor.getPercentDone();
                    if (zipPercent > percent) {
                        monitor.worked(zipPercent - percent);
                        percent = zipPercent;
                    }
                    monitor.update("Extracting... " + zipProgressMonitor.getPercentDone() + "%");
                    Thread.sleep(100);
                }

                Result result = zipProgressMonitor.getResult();
                if (result.equals(Result.SUCCESS)) {
                    monitor.done();
                } else {
                    monitor.failed(zipProgressMonitor.getException());
                }
            });

            if (zipProgressMonitor.getException() != null) {
                throw zipProgressMonitor.getException();
            }
            return tsvFile;
        }

    }

    private Path downloadZippedNotesFile(LocalDate date) throws IOException {
        int day = date.getDayOfMonth();
        int month = date.getMonthValue();
        int year = date.getYear();

        String filename = String.format("%04d-%02d-%02d-notes-00000.zip", year, month, day);
        Path filepath = getWorkdir().resolve(filename);

        if (!forceDownload && Files.exists(filepath)) {
            TaskService.monitor(SPINNER, TIME, TASK_NAME).run(monitor -> {
                monitor.update("File already exists: " + filepath);
                monitor.done();
            });
            return filepath;
        }

        String notesDataUrl = String.format("https://ton.twimg.com/birdwatch-public-data/%04d/%02d/%02d/notes/notes-00000.zip", year, month, day);

        log.debug("Downloading {} to {}...", notesDataUrl, filepath);
        DownloadTask downloadTask = new DownloadTask(notesDataUrl, filepath.toString());
        TaskService.monitor(Components.FILE).run(downloadTask);
        return filepath;
    }

    public static void main(String[] args) {
        Path workdir = Paths.get(".tmp");
        var grabber = new Grabber(workdir);
        grabber.run();
    }
}
